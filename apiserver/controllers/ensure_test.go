package controllers

import (
	"net/http"
	"testing"

	runnerParams "github.com/cloudbase/garm/params"
)

func TestNormalizeEnsureParams(t *testing.T) {
	cases := []struct {
		name      string
		in        runnerParams.EnsureEntityParams
		want      runnerParams.EnsureEntityParams
		wantError bool
	}{
		{
			name: "defaults",
			in:   runnerParams.EnsureEntityParams{Owner: "acme", Repo: "ci-test"},
			want: runnerParams.EnsureEntityParams{
				Forge: "gitea", Owner: "acme", Repo: "ci-test",
				Labels: []string{"garm-pools"}, MaxRunners: 2,
			},
		},
		{
			name: "trims and deduplicates labels",
			in: runnerParams.EnsureEntityParams{
				Forge: "Gitea", Owner: " acme ", Repo: " ci-test ",
				Labels: []string{" garm-pools ", "garm-pools", "", "linux"}, MaxRunners: 3,
			},
			want: runnerParams.EnsureEntityParams{
				Forge: "gitea", Owner: "acme", Repo: "ci-test",
				Labels: []string{"garm-pools", "linux"}, MaxRunners: 3,
			},
		},
		{
			name: "caps runners",
			in:   runnerParams.EnsureEntityParams{Owner: "acme", MaxRunners: 99},
			want: runnerParams.EnsureEntityParams{
				Forge: "gitea", Owner: "acme",
				Labels: []string{"garm-pools"}, MaxRunners: vosEnsureMaxRunners,
			},
		},
		{
			name:      "rejects other forges",
			in:        runnerParams.EnsureEntityParams{Forge: "github", Owner: "acme"},
			wantError: true,
		},
		{
			name:      "rejects path traversal in owner",
			in:        runnerParams.EnsureEntityParams{Owner: "../acme"},
			wantError: true,
		},
		{
			name:      "rejects path traversal in repo",
			in:        runnerParams.EnsureEntityParams{Owner: "acme", Repo: "a/b"},
			wantError: true,
		},
		{
			name: "rejects too many labels",
			in: runnerParams.EnsureEntityParams{Owner: "acme", Labels: []string{
				"a", "b", "c", "d", "e", "f", "g", "h", "i",
			}},
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeEnsureParams(tc.in)
			if tc.wantError {
				if err == nil {
					t.Fatalf("normalizeEnsureParams() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeEnsureParams() error = %v", err)
			}
			if got.Forge != tc.want.Forge || got.Owner != tc.want.Owner || got.Repo != tc.want.Repo || got.MaxRunners != tc.want.MaxRunners {
				t.Fatalf("normalizeEnsureParams() = %+v, want %+v", got, tc.want)
			}
			if len(got.Labels) != len(tc.want.Labels) {
				t.Fatalf("labels = %v, want %v", got.Labels, tc.want.Labels)
			}
			for i := range got.Labels {
				if got.Labels[i] != tc.want.Labels[i] {
					t.Fatalf("labels = %v, want %v", got.Labels, tc.want.Labels)
				}
			}
		})
	}
}

func TestVOSBearerToken(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/vos/ensure", nil)
	if got := vosBearerToken(req); got != "" {
		t.Fatalf("vosBearerToken() = %q, want empty", got)
	}
	req.Header.Set("Authorization", "bearer  abc123 ")
	if got := vosBearerToken(req); got != "abc123" {
		t.Fatalf("vosBearerToken() = %q, want abc123", got)
	}
	req.Header.Set("Authorization", "Basic abc123")
	if got := vosBearerToken(req); got != "" {
		t.Fatalf("vosBearerToken() = %q, want empty", got)
	}
}

func TestVOSPermissionURL(t *testing.T) {
	endpoint := runnerParams.ForgeEndpoint{
		Name:       "vos-gitea",
		BaseURL:    "http://172.17.0.1:1182/app/com.ictrek.gitea/",
		APIBaseURL: "http://172.17.0.1:1182/app/com.ictrek.gitea/",
	}

	orgURL := vosPermissionURL(endpoint, runnerParams.EnsureEntityParams{Owner: "acme"})
	if want := "http://172.17.0.1:1182/app/com.ictrek.gitea/api/v1/orgs/acme/hooks"; orgURL != want {
		t.Fatalf("vosPermissionURL() = %q, want %q", orgURL, want)
	}

	repoURL := vosPermissionURL(endpoint, runnerParams.EnsureEntityParams{Owner: "acme", Repo: "ci-test"})
	if want := "http://172.17.0.1:1182/app/com.ictrek.gitea/api/v1/repos/acme/ci-test"; repoURL != want {
		t.Fatalf("vosPermissionURL() = %q, want %q", repoURL, want)
	}

	if got := vosPermissionURL(runnerParams.ForgeEndpoint{}, runnerParams.EnsureEntityParams{Owner: "acme"}); got != "" {
		t.Fatalf("vosPermissionURL() = %q, want empty", got)
	}
}
