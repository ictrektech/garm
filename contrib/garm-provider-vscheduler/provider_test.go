package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeastLoadedSpread(t *testing.T) {
	candidates := []node{{ID: "b"}, {ID: "a"}}
	load := map[string]int{"a": 3, "b": 1}

	for i := 0; i < 5; i++ {
		if got := leastLoaded(candidates, load, "garm-"+string(rune('a'+i))); got.ID != "b" {
			t.Fatalf("picked %s, want the node with lower load", got.ID)
		}
	}

	first := leastLoaded(candidates, map[string]int{}, "same-name")
	second := leastLoaded(candidates, map[string]int{}, "same-name")
	if first.ID != second.ID {
		t.Fatal("tie break is not deterministic")
	}
}

func TestStatusOf(t *testing.T) {
	cases := map[string]string{
		"pending":        "running",
		"running":        "running",
		"stopped":        "stopped",
		"deleting":       "stopped",
		"removed":        "stopped",
		"failed":         "error",
		"cleanup_failed": "error",
	}
	for state, want := range cases {
		if got := statusOf(deployment{State: state}); got != want {
			t.Errorf("state %s: got %s, want %s", state, got, want)
		}
	}
}

func TestResolveTargetPicksOnlineNodeWithProfile(t *testing.T) {
	var created createRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/managed-nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing bearer token")
		}
		writeEnvelope(t, w, map[string]any{"items": []node{
			{ID: "n1", Status: "online", Profile: "jetson-thor", Arch: "aarch64", CompatibleProfiles: []string{"jetson-thor", "dgx-spark"}},
			{ID: "n2", Status: "offline", Profile: "jetson-thor", CompatibleProfiles: []string{"jetson-thor"}},
			{ID: "n3", Status: "online", Profile: "arm-cpu", CompatibleProfiles: []string{"arm-cpu"}},
		}})
	})
	mux.HandleFunc("/backend-deployments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("decoding create request: %v", err)
			}
			writeEnvelope(t, w, map[string]any{"id": "dep-1", "state": "pending", "request_key": created.RequestKey})
			return
		}
		writeEnvelope(t, w, map[string]any{"items": []deployment{
			{ID: "old", TargetID: "n1", State: "running", Config: map[string]any{"pool_id": "pool-1"}},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	api := newAPI(config{BaseURL: srv.URL, Token: "token", ContainerPort: 8080, TimeoutSeconds: 5})

	target, err := resolveTarget(api, poolSpecs{Profile: "jetson-thor"}, "garm-abc", "arm64")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if target.ID != "n1" || target.Kind != "ssh" || target.Profile != "jetson-thor" {
		t.Fatalf("unexpected target %+v", target)
	}

	dep, err := api.create(createRequest{
		RequestKey:    "garm-abc",
		Target:        target,
		Image:         "registry.example.com/runner:1.0",
		ContainerPort: 8080,
		Config:        map[string]any{"pool_id": "pool-1", "metadata_url": "http://garm/api/v1/metadata"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if dep.ID != "dep-1" {
		t.Fatalf("unexpected deployment %+v", dep)
	}
	if created.RequestKey != "garm-abc" || created.Target.ID != "n1" {
		t.Fatalf("unexpected create request %+v", created)
	}
	if created.Config["metadata_url"] != "http://garm/api/v1/metadata" {
		t.Fatalf("runner bootstrap config was not forwarded: %+v", created.Config)
	}
}

func TestRunnerConfigCarriesGiteaBootstrap(t *testing.T) {
	got := runnerConfig(bootstrap{
		Name:          "garm-abc",
		RepoURL:       "http://10.184.191.1:1182/gitea/api/v1",
		MetadataURL:   "http://garm/api/v1/metadata",
		InstanceToken: "jwt",
		Labels:        []string{"linux", "arm64", "runner-pool-id:pool-1"},
	}, "pool-1")

	want := map[string]string{
		"runner_name":    "garm-abc",
		"repo_url":       "http://10.184.191.1:1182/gitea/api/v1",
		"metadata_url":   "http://garm/api/v1/metadata",
		"instance_token": "jwt",
		"pool_id":        "pool-1",
		"labels":         "linux,arm64,runner-pool-id:pool-1",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("config[%q] = %v, want %q", key, got[key], value)
		}
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code": "OK", "msg": "OK", "data": data,
	}); err != nil {
		t.Errorf("encoding response: %v", err)
	}
}

func TestResolveImage(t *testing.T) {
	cases := []struct {
		name       string
		poolImage  string
		poolTag    string
		provider   string
		osArch     string
		want       string
		wantErrSub string
	}{
		{
			name:      "explicit tag is kept",
			poolImage: "swr.example.com/ci/garm-runner:arm_v6.0.1_3f9a1c2b",
			provider:  "v9.9.9_ignore",
			osArch:    "arm64",
			want:      "swr.example.com/ci/garm-runner:arm_v6.0.1_3f9a1c2b",
		},
		{
			name:      "digest is kept",
			poolImage: "swr.example.com/ci/garm-runner@sha256:" + strings.Repeat("a", 64),
			osArch:    "arm64",
			want:      "swr.example.com/ci/garm-runner@sha256:" + strings.Repeat("a", 64),
		},
		{
			name:      "arm pool gets the arm prefix",
			poolImage: "swr.example.com/ci/garm-runner",
			provider:  "v6.0.1_3f9a1c2b",
			osArch:    "arm64",
			want:      "swr.example.com/ci/garm-runner:arm_v6.0.1_3f9a1c2b",
		},
		{
			name:      "amd pool gets the amd prefix",
			poolImage: "swr.example.com/ci/garm-runner",
			provider:  "v6.0.1_3f9a1c2b",
			osArch:    "amd64",
			want:      "swr.example.com/ci/garm-runner:amd_v6.0.1_3f9a1c2b",
		},
		{
			name:      "registry port is not a tag",
			poolImage: "10.0.0.5:5000/ci/garm-runner",
			provider:  "v6.0.1_3f9a1c2b",
			osArch:    "arm64",
			want:      "10.0.0.5:5000/ci/garm-runner:arm_v6.0.1_3f9a1c2b",
		},
		{
			name:      "pool override wins over the provider default",
			poolImage: "swr.example.com/ci/garm-runner",
			poolTag:   "v6.0.2_aaaa1111",
			provider:  "v6.0.1_3f9a1c2b",
			osArch:    "arm64",
			want:      "swr.example.com/ci/garm-runner:arm_v6.0.2_aaaa1111",
		},
		{
			name:       "no tag anywhere",
			poolImage:  "swr.example.com/ci/garm-runner",
			osArch:     "arm64",
			wantErrSub: "has no tag",
		},
		{
			name:       "unknown architecture",
			poolImage:  "swr.example.com/ci/garm-runner",
			provider:   "v6.0.1_3f9a1c2b",
			osArch:     "",
			wantErrSub: "os_arch",
		},
	}

	for _, tc := range cases {
		got, err := resolveImage(tc.poolImage, tc.poolTag, tc.provider, tc.osArch)
		if tc.wantErrSub != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("%s: got (%q, %v), want error containing %q", tc.name, got, err, tc.wantErrSub)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveTargetRejectsArchMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/managed-nodes", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, map[string]any{"items": []node{
			{ID: "n1", Status: "online", Profile: "jetson-thor", Arch: "aarch64",
				CompatibleProfiles: []string{"jetson-thor"}},
		}})
	})
	mux.HandleFunc("/backend-deployments", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, map[string]any{"items": []deployment{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	api := newAPI(config{BaseURL: srv.URL, Token: "token", ContainerPort: 8080, TimeoutSeconds: 5})

	_, err := resolveTarget(api, poolSpecs{Profile: "jetson-thor"}, "garm-abc", "amd64")
	if err == nil || !strings.Contains(err.Error(), "does not match node arch") {
		t.Fatalf("got %v, want an architecture mismatch error", err)
	}
}

func TestEntrypointReadsEveryBootstrapKey(t *testing.T) {
	// The provider writes the deploy config keys, entrypoint.sh reads them back.
	// Renaming one side must fail this test.
	script, err := os.ReadFile(filepath.Join("runner-image", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("reading entrypoint.sh: %v", err)
	}
	cfg := runnerConfig(bootstrap{
		Name: "garm-abc", RepoURL: "http://gitea/api/v1",
		MetadataURL: "http://garm/metadata", CallbackURL: "http://garm/callbacks",
		InstanceToken: "jwt", Labels: []string{"linux"},
	}, "pool-1")
	for key := range cfg {
		if key == "pool_id" {
			// pool_id is only used by the provider to filter deployments.
			continue
		}
		if !strings.Contains(string(script), key) {
			t.Errorf("entrypoint.sh never reads %q, which the provider sends", key)
		}
	}
}

func TestResolveImageRejectsPrefixedTag(t *testing.T) {
	_, err := resolveImage("swr.example.com/ci/garm-runner", "", "arm_v6.0.1_3f9a1c2b", "arm64")
	if err == nil || !strings.Contains(err.Error(), "architecture prefix") {
		t.Fatalf("got %v, want an architecture prefix error", err)
	}
}
