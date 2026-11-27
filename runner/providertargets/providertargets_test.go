package providertargets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig puts a provider config file into a temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestListReturnsManagedNodes(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/managed-nodes" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		// V-Scheduler wraps every answer in an envelope.
		_, _ = w.Write([]byte(`{"code":"OK","data":{"items":[
			{"id":"01M88Y1W","name":"thor35","status":"online","profile":"jetson-thor",
			 "arch":"aarch64"}
		]}}`))
	}))
	defer server.Close()

	path := writeConfig(t, `{"base_url":"`+server.URL+`","token":"secret",`+
		`"image_repository":"registry.example.com/ci/runner","image_tag":"v6.0.1_27ec9940"}`)
	info, err := List(context.Background(), path)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("authorization header = %q", gotAuth)
	}
	if len(info.Targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(info.Targets))
	}
	got := info.Targets[0]
	if got.ID != "01M88Y1W" || got.Name != "thor35" || got.Profile != "jetson-thor" || got.Arch != "aarch64" {
		t.Errorf("unexpected target: %+v", got)
	}
	if info.ImageRepository != "registry.example.com/ci/runner" || info.ImageTag != "v6.0.1_27ec9940" {
		t.Errorf("image suggestion not carried over: %+v", info)
	}
}

func TestListWithoutCredentialsStillSuggestsTheImage(t *testing.T) {
	path := writeConfig(t, `{"image_repository":"registry.example.com/ci/runner","image_tag":"v1"}`)
	info, err := List(context.Background(), path)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(info.Targets) != 0 {
		t.Errorf("got %d targets, want none", len(info.Targets))
	}
	if info.ImageRepository != "registry.example.com/ci/runner" {
		t.Errorf("image repository lost: %+v", info)
	}
}

func TestListWithEmptyConfigPathIsEmpty(t *testing.T) {
	info, err := List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(info.Targets) != 0 || info.ImageRepository != "" || info.ImageTag != "" {
		t.Errorf("got %+v, want an empty answer", info)
	}
}

func TestListTreatsNonJSONConfigAsNoTargets(t *testing.T) {
	// The LXD provider uses a TOML config. Decoding it must not fail: the pool
	// form asks every provider for targets.
	info, err := List(context.Background(), writeConfig(t, "provider_executable = \"/opt/x\"\n"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(info.Targets) != 0 {
		t.Errorf("got %+v, want an empty answer", info)
	}
}

func TestListTreatsMissingConfigAsEmpty(t *testing.T) {
	info, err := List(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(info.Targets) != 0 {
		t.Errorf("got %+v, want an empty answer", info)
	}
}

func TestListKeepsImageSuggestionWhenTheNodeListFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	path := writeConfig(t, `{"base_url":"`+server.URL+`","token":"bad",`+
		`"image_repository":"registry.example.com/ci/runner"}`)
	info, err := List(context.Background(), path)
	if err == nil {
		t.Error("expected an error for a 401 answer")
	}
	if info.ImageRepository != "registry.example.com/ci/runner" {
		t.Errorf("image repository lost on error: %+v", info)
	}
}

func TestListReportsUnreachableServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	path := writeConfig(t, `{"base_url":"`+url+`","token":"secret"}`)
	if _, err := List(context.Background(), path); err == nil {
		t.Error("expected an error for a closed server")
	}
}

func TestListAcceptsATimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"OK","data":{"items":[]}}`))
	}))
	defer server.Close()

	path := writeConfig(t, `{"base_url":"`+server.URL+`","token":"t","timeout_seconds":3}`)
	if _, err := List(context.Background(), path); err != nil {
		t.Fatalf("List: %v", err)
	}
}
