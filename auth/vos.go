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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

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

type vosIdentity struct {
	Subject  string
	Username string
	FullName string
	IsAdmin  bool
	Enabled  bool
}

func vosAuthEnabled() bool {
	return os.Getenv("GARM_VOS_AUTH_ENABLED") == "true"
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
	username := claimValue(claims, "preferred_username", "username")
	if subject == "" || len(subject) > 128 {
		return vosIdentity{}, errors.New("userinfo subject is missing or too long")
	}
	if username == "" {
		return vosIdentity{}, errors.New("userinfo is missing username")
	}

	// VOS application tokens use the application ID for both fields. They
	// are service identities even when the gateway reports an admin role.
	if subject == username {
		return vosIdentity{}, errors.New("userinfo is not a VOS user account")
	}
	if len(username) > maxUsernameLength {
		return vosIdentity{}, errors.New("userinfo username exceeds GARM's account name limit")
	}

	identity := vosIdentity{
		Subject:  subject,
		Username: username,
		FullName: claimValue(claims, "nickname", "name", "preferred_username", "username"),
		IsAdmin:  claimValue(claims, "role_code") == "system_admin",
		Enabled:  claims["enable"] == float64(1),
	}
	if identity.FullName == "" {
		identity.FullName = username
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

	client := &http.Client{Timeout: vosUserInfoTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
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

// AuthenticateVOSUser validates an enabled VOS account and synchronizes its GARM identity.
func (a *Authenticator) AuthenticateVOSUser(ctx context.Context, accessToken, userinfoURL string) (context.Context, runnerParams.User, error) {
	if !vosAuthEnabled() || strings.TrimSpace(accessToken) == "" {
		return ctx, runnerParams.User{}, runnerErrors.ErrUnauthorized
	}
	if userinfoURL == "" {
		userinfoURL = DefaultVOSUserInfoURL
	}
	identity, err := fetchVOSIdentity(ctx, accessToken, userinfoURL)
	if err != nil || !identity.Enabled {
		return ctx, runnerParams.User{}, runnerErrors.ErrUnauthorized
	}

	// VOS accounts authenticate only through SSO. Derive an internal unique
	// email from the subject so shared or changed email addresses cannot link accounts.
	digest := sha256.Sum256([]byte(identity.Subject))
	user, err := a.store.SyncVOSUser(ctx, identity.Subject, runnerParams.NewUserParams{
		Username: identity.Username,
		Email:    fmt.Sprintf("%x@vos.invalid", digest),
		FullName: identity.FullName,
		IsAdmin:  identity.IsAdmin,
		Enabled:  true,
	})
	if err != nil {
		return ctx, runnerParams.User{}, err
	}
	return PopulateContext(ctx, user, nil), user, nil
}
