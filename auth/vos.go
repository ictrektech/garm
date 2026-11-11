// Copyright 2022 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	runnerErrors "github.com/cloudbase/garm-provider-common/errors"
	runnerParams "github.com/cloudbase/garm/params"
)

const (
	// DefaultVOSUserInfoURL is the VOS OIDC userinfo endpoint reachable from
	// applications running on the VOS host network.
	DefaultVOSUserInfoURL = "http://172.17.0.1:8105/v1000/oauth2/userinfo"

	// DefaultVOSClientID is the OAuth2 client ID declared in ictrek.app/manifest.yml.
	DefaultVOSClientID = "com.ictrek.garm"

	vosUserInfoTimeout = 10 * time.Second
	maxVOSUserInfoSize = 64 * 1024
	maxUsernameLength  = 64
)

// vosUsernameUnsafe matches everything the GARM users table does not accept.
var vosUsernameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

type vosIdentity struct {
	Subject  string
	Username string
	Email    string
	FullName string
}

// VOSUserInfoURL returns the configured userinfo endpoint.
func VOSUserInfoURL() string {
	return DefaultVOSUserInfoURL
}

func claimValue(claims map[string]any, names ...string) string {
	for _, name := range names {
		switch value := claims[name].(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case float64:
			return fmt.Sprintf("%.0f", value)
		}
	}
	return ""
}

func sanitizeVOSUsername(raw string) (string, error) {
	name := vosUsernameUnsafe.ReplaceAllString(raw, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "", fmt.Errorf("empty username after sanitizing %q", raw)
	}
	if len(name) > maxUsernameLength {
		name = name[:maxUsernameLength]
	}
	return name, nil
}

func parseVOSUserInfo(body []byte) (vosIdentity, error) {
	var envelope struct {
		Code *float64        `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return vosIdentity{}, fmt.Errorf("invalid userinfo response: %w", err)
	}
	if envelope.Code != nil && *envelope.Code != 0 {
		return vosIdentity{}, fmt.Errorf("VOS rejected the access token (code %.0f)", *envelope.Code)
	}

	payload := body
	if len(envelope.Data) > 0 {
		payload = envelope.Data
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return vosIdentity{}, fmt.Errorf("invalid userinfo claims: %w", err)
	}

	subject := claimValue(claims, "sub", "id", "user_id")
	username := claimValue(claims, "preferred_username", "username", "nickname", "name", "email")
	if subject == "" {
		return vosIdentity{}, errors.New("userinfo is missing subject")
	}
	if username == "" {
		return vosIdentity{}, errors.New("userinfo is missing username")
	}

	safeUsername, err := sanitizeVOSUsername(username)
	if err != nil {
		return vosIdentity{}, err
	}

	identity := vosIdentity{
		Subject:  subject,
		Username: safeUsername,
		Email:    claimValue(claims, "email"),
		FullName: claimValue(claims, "name", "preferred_username", "username", "nickname"),
	}
	if identity.FullName == "" {
		identity.FullName = safeUsername
	}
	return identity, nil
}

func fetchVOSIdentity(ctx context.Context, accessToken, userinfoURL string) (vosIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return vosIdentity{}, fmt.Errorf("building userinfo request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: vosUserInfoTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return vosIdentity{}, fmt.Errorf("calling VOS userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVOSUserInfoSize))
	if err != nil {
		return vosIdentity{}, fmt.Errorf("reading VOS userinfo: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return vosIdentity{}, fmt.Errorf("VOS userinfo returned %d", resp.StatusCode)
	}
	return parseVOSUserInfo(body)
}

func randomPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// AuthenticateVOSUser validates a VOS OIDC access token and returns a context
// populated with the matching GARM user. The first VOS user becomes the GARM
// administrator so that a fresh deployment can be used without the first-run
// wizard.
func (a *Authenticator) AuthenticateVOSUser(ctx context.Context, accessToken, userinfoURL string) (context.Context, error) {
	if strings.TrimSpace(accessToken) == "" {
		return ctx, runnerErrors.ErrUnauthorized
	}
	if userinfoURL == "" {
		userinfoURL = DefaultVOSUserInfoURL
	}

	identity, err := fetchVOSIdentity(ctx, accessToken, userinfoURL)
	if err != nil {
		return ctx, runnerErrors.ErrUnauthorized
	}

	user, err := a.store.GetUser(ctx, identity.Username)
	if err != nil {
		if !errors.Is(err, runnerErrors.ErrNotFound) {
			return ctx, fmt.Errorf("looking up user %s: %w", identity.Username, err)
		}
		user, err = a.createVOSUser(ctx, identity)
		if err != nil {
			return ctx, err
		}
	}

	if !user.Enabled {
		return ctx, runnerErrors.ErrUnauthorized
	}
	return PopulateContext(ctx, user, nil), nil
}

func (a *Authenticator) createVOSUser(ctx context.Context, identity vosIdentity) (runnerParams.User, error) {
	password, err := randomPassword()
	if err != nil {
		return runnerParams.User{}, err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return runnerParams.User{}, fmt.Errorf("hashing password: %w", err)
	}

	email := strings.TrimSpace(identity.Email)
	if email == "" {
		email = fmt.Sprintf("%s@garm.local", identity.Username)
	}

	newUser := runnerParams.NewUserParams{
		Username: identity.Username,
		Email:    email,
		FullName: identity.FullName,
		Password: string(hashed),
		IsAdmin:  !a.store.HasAdminUser(ctx),
		Enabled:  true,
	}
	return a.store.CreateUser(ctx, newUser)
}
