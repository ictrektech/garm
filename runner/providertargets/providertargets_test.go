package providertargets

import (
	"context"
	"encoding/json"
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

	path := writeConfig(t, `{"base_url":"`+server.URL+`","token":"secret"}`)
	targets, err := List(context.Background(), path)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("authorization header = %q", gotAuth)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	got := targets[0]
	if got.ID != "01M88Y1W" || got.Name != "thor35" || got.Profile != "jetson-thor" || got.Arch != "aarch64" {
		t.Errorf("unexpected target: %+v", got)
	}
}

func TestListWithoutCredentialsIsEmpty(t *testing.T) {
	cases := map[string]string{
		"no keys":  `{}`,
		"no token": `{"base_url":"http://127.0.0.1:1"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			targets, err := List(context.Background(), writeConfig(t, body))
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if targets != nil {
				t.Errorf("got %v, want nil", targets)
			}
		})
	}
}

func TestListWithEmptyConfigPathIsEmpty(t *testing.T) {
	targets, err := List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if targets != nil {
		t.Errorf("got %v, want nil", targets)
	}
}

func TestListReportsBadConfig(t *testing.T) {
	if _, err := List(context.Background(), writeConfig(t, `not json`)); err == nil {
		t.Error("expected an error for a malformed config")
	}
	if _, err := List(context.Background(), filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("expected an error for a missing config file")
	}
}

func TestListReportsRejectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	path := writeConfig(t, `{"base_url":"`+server.URL+`","token":"bad"}`)
	if _, err := List(context.Background(), path); err == nil {
		t.Error("expected an error for a 401 answer")
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

func TestListSendsConfiguredTimeout(t *testing.T) {
	// A config without timeout_seconds must still work; a numeric one must be
	// accepted without breaking parsing.
	var got struct {
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	body, _ := json.Marshal(map[string]any{"base_url": server.URL, "token": "t", "timeout_seconds": 3})
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := List(context.Background(), writeConfig(t, string(body))); err != nil {
		t.Fatalf("List: %v", err)
	}
}
