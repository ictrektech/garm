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
	"testing"

	"github.com/stretchr/testify/require"
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
