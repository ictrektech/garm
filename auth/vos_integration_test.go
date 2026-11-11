//go:build testing

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

package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/config"
	"github.com/cloudbase/garm/database/sql"
	"github.com/cloudbase/garm/database/watcher"
	garmTesting "github.com/cloudbase/garm/internal/testing"
	"github.com/cloudbase/garm/params"
)

func TestVOSAccountSync(t *testing.T) {
	t.Setenv("GARM_VOS_AUTH_ENABLED", "true")
	ctx := context.Background()
	watcher.InitWatcher(ctx)
	t.Cleanup(func() { _ = watcher.CloseWatcher() })
	db, err := sql.NewSQLDatabase(ctx, garmTesting.GetTestSqliteDBConfig(t))
	require.NoError(t, err)
	cfg := config.JWTAuth{Secret: "test-only-signing-secret"}
	authenticator := auth.NewAuthenticator(cfg, db)
	middleware, err := auth.NewjwtMiddleware(db, cfg)
	require.NoError(t, err)
	status := func(user params.User) int {
		claims := auth.JWTClaims{UserID: user.ID, Generation: user.Generation, IsAdmin: user.IsAdmin}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.Secret))
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()
		middleware.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(resp, req)
		return resp.Code
	}
	login := func(subject, username, role string, enabled int) (context.Context, params.User, error) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub": subject, "preferred_username": username, "role_code": role, "enable": enabled,
			})
		}))
		defer server.Close()
		return authenticator.AuthenticateVOSUser(ctx, "verified-by-test-server", server.URL)
	}

	ordinary, member, err := login("member-id", "member", "member", 1)
	require.NoError(t, err)
	require.False(t, auth.IsAdmin(ordinary))
	require.False(t, db.HasAdminUser(ctx))
	require.Equal(t, "member", member.Username)

	legacy, err := db.CreateUser(ctx, params.NewUserParams{
		Username: "legacy", Email: "legacy@example.com", Password: "old-hash", Enabled: true, IsAdmin: true,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, status(legacy), "pre-upgrade admin sessions must be rejected")

	adminCtx, admin, err := login("admin-id", "admin", "system_admin", 1)
	require.NoError(t, err)
	require.True(t, auth.IsAdmin(adminCtx))
	secondCtx, second, err := login("second-id", "ops.admin", "system_admin", 1)
	require.NoError(t, err)
	require.True(t, auth.IsAdmin(secondCtx))
	require.Equal(t, "ops.admin", second.Username)

	_, migrated, err := login("legacy-sub", "legacy", "member", 1)
	require.NoError(t, err)
	require.NotEqual(t, legacy.ID, migrated.ID)
	require.Equal(t, "legacy", migrated.Username)
	oldRecord, err := db.GetUserByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.False(t, oldRecord.Enabled)

	downgradedCtx, downgraded, err := login("second-id", "ops.renamed", "member", 1)
	require.NoError(t, err)
	require.Equal(t, second.ID, downgraded.ID)
	require.Equal(t, "ops.renamed", downgraded.Username)
	require.False(t, auth.IsAdmin(downgradedCtx))
	require.Equal(t, http.StatusUnauthorized, status(second), "old admin session must be revoked after role sync")
	require.Equal(t, http.StatusNoContent, status(admin))

	_, _, err = login("other-subject", "ops.renamed", "system_admin", 1)
	require.Error(t, err, "a subject cannot take over another bound account")
	_, _, err = login("com.ictrek.example", "com.ictrek.example", "system_admin", 1)
	require.Error(t, err, "application tokens cannot enroll admin accounts")
	_, _, err = login("disabled-id", "disabled", "system_admin", 0)
	require.Error(t, err, "disabled VOS users cannot sign in")
	_, err = authenticator.InitController(ctx, params.NewUserParams{})
	require.Error(t, err, "standalone initialization is unavailable in VOS")
}
