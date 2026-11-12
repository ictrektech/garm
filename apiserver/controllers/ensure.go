// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	gErrors "github.com/cloudbase/garm-provider-common/errors"

	"github.com/cloudbase/garm/auth"
	runnerParams "github.com/cloudbase/garm/params"
)

const (
	vosEnsureDefaultMaxRunners = 2
	vosEnsureMaxRunners        = 4
	vosEnsureMaxLabels         = 8
	vosEnsureMaxLabelLength    = 50
	vosEnsureDefaultLabel      = "garm-pools"
)

// Gitea owner and repository names.
var vosForgeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// VOSEnsureHandler provisions a Gitea organization or repository in GARM.
//
// Gitea calls this after the user asks to register the entity. The token is a
// VOS access token, so GARM can check the caller against VOS and Gitea.
func (a *APIController) VOSEnsureHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var param runnerParams.EnsureEntityParams
	if err := json.NewDecoder(r.Body).Decode(&param); err != nil {
		handleError(ctx, w, gErrors.ErrBadRequest)
		return
	}

	param, err := normalizeEnsureParams(param)
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	userCtx, user, err := a.auth.AuthenticateVOSUser(ctx, vosBearerToken(r), vosUserInfoURL())
	if err != nil {
		slog.ErrorContext(ctx, "VOS ensure authentication failed", "error", err)
		handleError(ctx, w, gErrors.ErrUnauthorized)
		return
	}
	// Until the Gitea permission check lands, only VOS administrators register
	// entities.
	if !user.IsAdmin {
		handleError(ctx, w, gErrors.ErrUnauthorized)
		return
	}

	adminCtx := auth.SetAdmin(userCtx, true)
	result, err := a.r.EnsureGiteaEntity(adminCtx, param)
	if err != nil {
		slog.ErrorContext(ctx, "VOS ensure failed", "owner", param.Owner, "repo", param.Repo, "error", err)
		handleError(ctx, w, err)
		return
	}

	info, err := a.r.GetControllerInfo(adminCtx)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	result.WebhookURL = strings.TrimRight(info.ControllerWebhookURL, "/")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		handleError(ctx, w, err)
	}
}

// vosBearerToken returns the credential of a Bearer Authorization header.
func vosBearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < len("Bearer ") || !strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[len("Bearer "):])
}

// normalizeEnsureParams validates the request and applies the VOS limits.
func normalizeEnsureParams(param runnerParams.EnsureEntityParams) (runnerParams.EnsureEntityParams, error) {
	param.Forge = strings.ToLower(strings.TrimSpace(param.Forge))
	if param.Forge == "" {
		param.Forge = "gitea"
	}
	if param.Forge != "gitea" {
		return param, gErrors.NewBadRequestError("unsupported forge %q", param.Forge)
	}

	param.Owner = strings.TrimSpace(param.Owner)
	param.Repo = strings.TrimSpace(param.Repo)
	if !vosForgeName.MatchString(param.Owner) {
		return param, gErrors.NewBadRequestError("invalid owner name")
	}
	if param.Repo != "" && !vosForgeName.MatchString(param.Repo) {
		return param, gErrors.NewBadRequestError("invalid repository name")
	}

	labels := make([]string, 0, len(param.Labels))
	seen := map[string]bool{}
	for _, label := range param.Labels {
		label = strings.TrimSpace(label)
		if label == "" || seen[label] {
			continue
		}
		if len(label) > vosEnsureMaxLabelLength {
			return param, gErrors.NewBadRequestError("label %q is longer than %d characters", label, vosEnsureMaxLabelLength)
		}
		if len(labels) == vosEnsureMaxLabels {
			return param, gErrors.NewBadRequestError("more than %d labels", vosEnsureMaxLabels)
		}
		seen[label] = true
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		labels = []string{vosEnsureDefaultLabel}
	}
	param.Labels = labels

	if param.MaxRunners == 0 {
		param.MaxRunners = vosEnsureDefaultMaxRunners
	}
	if param.MaxRunners > vosEnsureMaxRunners {
		param.MaxRunners = vosEnsureMaxRunners
	}
	return param, nil
}
