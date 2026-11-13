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

package controllers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	gErrors "github.com/cloudbase/garm-provider-common/errors"
	"github.com/cloudbase/garm/auth"
	runnerParams "github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner"
)

type vosLoginParams struct {
	AccessToken string `json:"access_token"`
}

func vosUserInfoURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("GARM_VOS_USERINFO_URL")); endpoint != "" {
		return endpoint
	}
	return auth.DefaultVOSUserInfoURL
}

// VOSLoginHandler exchanges a VOS OIDC access token for a GARM JWT.
//
// The endpoint is deliberately not guarded by the initialization middleware:
// a fresh deployment provisions users directly from VOS.
func (a *APIController) VOSLoginHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var loginInfo vosLoginParams
	if err := json.NewDecoder(r.Body).Decode(&loginInfo); err != nil {
		handleError(ctx, w, gErrors.ErrBadRequest)
		return
	}
	if strings.TrimSpace(loginInfo.AccessToken) == "" {
		handleError(ctx, w, gErrors.ErrBadRequest)
		return
	}

	ctx, user, err := a.auth.AuthenticateVOSUser(ctx, loginInfo.AccessToken, vosUserInfoURL())
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	if user.IsAdmin {
		if vosBrowserOrigin(r) != "" {
			a.configureVOSIntegration(ctx)
		}
	}

	tokenString, err := a.auth.GetJWTToken(ctx)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		runnerParams.JWTResponse
		Username string `json:"username"`
	}{JWTResponse: runnerParams.JWTResponse{Token: tokenString}, Username: user.Username}); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}

const vosLocalGitea = "http://gitea:8080"
const vosAppPath = "/app/com.ictrek.garm"

var vosIntegrationMu sync.Mutex

func vosBrowserOrigin(r *http.Request) string {
	u, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
		!strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" ||
		u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func vosGiteaAvailable(ctx context.Context, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16))
	return err == nil && resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "OK"
}

func (a *APIController) configureVOSIntegration(ctx context.Context) {
	vosIntegrationMu.Lock()
	defer vosIntegrationMu.Unlock()

	info, err := a.r.GetControllerInfo(ctx)
	if err == nil {
		base := runner.VOSInternalBaseURL() + vosAppPath
		missing := func(current, path string) *string {
			if current != "" {
				return nil
			}
			value := base + path
			return &value
		}
		update := runnerParams.UpdateControllerParams{
			MetadataURL: missing(info.MetadataURL, "/api/v1/metadata"),
			CallbackURL: missing(info.CallbackURL, "/api/v1/callbacks"),
			WebhookURL:  missing(info.WebhookURL, "/webhooks"),
			AgentURL:    missing(info.AgentURL, "/agent"),
		}
		if update.MetadataURL != nil || update.CallbackURL != nil || update.WebhookURL != nil || update.AgentURL != nil {
			if _, err := a.r.UpdateController(ctx, update); err != nil {
				slog.WarnContext(ctx, "Unable to initialize VOS controller URLs", "error", err)
			}
		}
	}

	endpoints, err := a.r.ListGiteaEndpoints(ctx)
	if err != nil {
		return
	}
	// Gitea is reached over the same internal gateway, both by GARM and by the
	// runner instances. vosLocalGitea is only used for the reachability probe.
	giteaURL := runner.VOSGiteaURL()
	for _, endpoint := range endpoints {
		if endpoint.Name == "vos-gitea" || strings.TrimRight(endpoint.BaseURL, "/") == strings.TrimRight(giteaURL, "/") ||
			strings.TrimRight(endpoint.APIBaseURL, "/") == strings.TrimRight(giteaURL, "/") {
			return
		}
	}
	if !vosGiteaAvailable(ctx, vosLocalGitea) {
		return
	}
	_, err = a.r.CreateGiteaEndpoint(ctx, runnerParams.CreateGiteaEndpointParams{
		Name: "vos-gitea", Description: "Gitea on this VOS device",
		BaseURL: giteaURL, APIBaseURL: giteaURL,
	})
	if err != nil {
		slog.WarnContext(ctx, "Unable to add the local VOS Gitea endpoint", "error", err)
	}
}
