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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm/config"
	"github.com/cloudbase/garm/database/sql"
	"github.com/cloudbase/garm/database/watcher"
	garmTesting "github.com/cloudbase/garm/internal/testing"
	"github.com/cloudbase/garm/params"
)

func TestParseVOSUserInfoNested(t *testing.T) {
	body := []byte(`{"code":0,"data":{"sub":"user-1","preferred_username":"alice","enable":1,"role_code":"system_admin","name":"Alice"}}`)
	identity, err := parseVOSUserInfo(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Subject != "user-1" {
		t.Errorf("subject = %q, want user-1", identity.Subject)
	}
	if identity.Username != "alice" {
		t.Errorf("username = %q, want alice", identity.Username)
	}
	require.True(t, identity.IsAdmin)
	require.True(t, identity.Enabled)
	if identity.FullName != "Alice" {
		t.Errorf("full name = %q, want Alice", identity.FullName)
	}
}

func TestParseVOSUserInfoFlat(t *testing.T) {
	body := []byte(`{"sub":"user-2","username":"bob"}`)
	identity, err := parseVOSUserInfo(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Username != "bob" {
		t.Errorf("username = %q, want bob", identity.Username)
	}
	if identity.FullName != "bob" {
		t.Errorf("full name = %q, want bob", identity.FullName)
	}
}

func TestParseVOSUserInfoRejected(t *testing.T) {
	body := []byte(`{"code":401,"data":{}}`)
	if _, err := parseVOSUserInfo(body); err == nil {
		t.Fatal("expected an error for a rejected token")
	}
}

func TestParseVOSUserInfoMissingUsername(t *testing.T) {
	body := []byte(`{"sub":"user-3"}`)
	if _, err := parseVOSUserInfo(body); err == nil {
		t.Fatal("expected an error when username is missing")
	}
}

func TestVOSAccountSync(t *testing.T) {
	t.Setenv("GARM_VOS_AUTH_ENABLED", "true")
	ctx := context.Background()
	watcher.InitWatcher(ctx)
	t.Cleanup(watcher.CloseWatcher)
	db, err := sql.NewSQLDatabase(ctx, garmTesting.GetTestSqliteDBConfig(t))
	require.NoError(t, err)
	auth := NewAuthenticator(config.JWTAuth{}, db)
	middleware := jwtMiddleware{store: db}
	login := func(subject, username, role string, enabled int) (context.Context, params.User, error) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub": subject, "preferred_username": username, "role_code": role, "enable": enabled,
			})
		}))
		defer server.Close()
		return auth.AuthenticateVOSUser(ctx, "verified-by-test-server", server.URL)
	}

	ordinary, member, err := login("member-id", "member", "member", 1)
	require.NoError(t, err)
	require.False(t, IsAdmin(ordinary))
	require.False(t, db.HasAdminUser(ctx))
	require.Equal(t, "member", member.Username)

	legacy, err := db.CreateUser(ctx, params.NewUserParams{
		Username: "legacy", Email: "legacy@example.com", Password: "old-hash", Enabled: true, IsAdmin: true,
	})
	require.NoError(t, err)
	_, err = middleware.claimsToContext(ctx, &JWTClaims{UserID: legacy.ID, Generation: legacy.Generation, IsAdmin: true})
	require.Error(t, err, "pre-upgrade sessions must be rejected")

	adminCtx, admin, err := login("admin-id", "admin", "system_admin", 1)
	require.NoError(t, err)
	require.True(t, IsAdmin(adminCtx))
	secondCtx, second, err := login("second-id", "ops.admin", "system_admin", 1)
	require.NoError(t, err)
	require.True(t, IsAdmin(secondCtx))
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
	require.False(t, IsAdmin(downgradedCtx))
	_, err = middleware.claimsToContext(ctx, &JWTClaims{UserID: second.ID, Generation: second.Generation, IsAdmin: true})
	require.Error(t, err, "old admin session must be revoked after role sync")
	_, err = middleware.claimsToContext(ctx, &JWTClaims{UserID: admin.ID, Generation: admin.Generation})
	require.NoError(t, err)

	_, _, err = login("other-subject", "ops.renamed", "system_admin", 1)
	require.Error(t, err, "a subject cannot take over another bound account")
	_, _, err = login("com.ictrek.example", "com.ictrek.example", "system_admin", 1)
	require.Error(t, err, "application tokens cannot enroll admin accounts")
	_, _, err = login("disabled-id", "disabled", "system_admin", 0)
	require.Error(t, err, "disabled VOS users cannot sign in")
	_, err = auth.InitController(ctx, params.NewUserParams{})
	require.Error(t, err, "standalone initialization is unavailable in VOS")
}
