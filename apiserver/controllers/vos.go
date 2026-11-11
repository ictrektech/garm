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
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	gErrors "github.com/cloudbase/garm-provider-common/errors"
	"github.com/cloudbase/garm/auth"
	runnerParams "github.com/cloudbase/garm/params"
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
// on a fresh deployment the first VOS user becomes the GARM administrator.
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

	ctx, err := a.auth.AuthenticateVOSUser(ctx, loginInfo.AccessToken, vosUserInfoURL())
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	tokenString, err := a.auth.GetJWTToken(ctx)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(runnerParams.JWTResponse{Token: tokenString}); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}
