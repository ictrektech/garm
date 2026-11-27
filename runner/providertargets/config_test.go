package providertargets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudbase/garm/params"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestReadRunnerConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	cfg, err := ReadRunnerConfig(path)
	if err != nil {
		t.Fatalf("ReadRunnerConfig: %v", err)
	}
	if !cfg.Supported || cfg.BaseURL != "" || cfg.TokenConfigured {
		t.Fatalf("unexpected answer for a missing file: %+v", cfg)
	}
}

func TestReadRunnerConfigNonJSONProvider(t *testing.T) {
	path := writeConfigFile(t, "unix_socket_path = \"/var/snap/lxd/common/lxd/unix.socket\"\n")
	cfg, err := ReadRunnerConfig(path)
	if err != nil {
		t.Fatalf("ReadRunnerConfig: %v", err)
	}
	if cfg.Supported {
		t.Fatalf("a TOML provider must not report runner support: %+v", cfg)
	}
}

func TestUpdateRunnerConfigCreatesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "provider.json")
	cfg, err := UpdateRunnerConfig(path, params.UpdateProviderRunnerConfigParams{
		BaseURL:         " http://v-scheduler:38120/api/v1/ ",
		Token:           "secret-token",
		ImageRepository: "registry.example.com/ci/runner",
		ImageTag:        "v6.0.1_27ec9940",
	})
	if err != nil {
		t.Fatalf("UpdateRunnerConfig: %v", err)
	}
	if !cfg.Supported || !cfg.TokenConfigured {
		t.Fatalf("unexpected answer: %+v", cfg)
	}
	if cfg.BaseURL != "http://v-scheduler:38120/api/v1" {
		t.Fatalf("base_url not normalized: %q", cfg.BaseURL)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("config is not JSON: %v", err)
	}
	if values["container_port"] != float64(8080) {
		t.Fatalf("container_port = %v, want 8080", values["container_port"])
	}
	if values["token"] != "secret-token" {
		t.Fatalf("token = %v", values["token"])
	}
}

func TestUpdateRunnerConfigKeepsStoredTokenAndUnknownKeys(t *testing.T) {
	path := writeConfigFile(t, `{"base_url":"http://old:38120/api/v1","token":"stored","custom":"keep"}`)
	cfg, err := UpdateRunnerConfig(path, params.UpdateProviderRunnerConfigParams{
		BaseURL:         "http://v-scheduler:38120/api/v1",
		ImageRepository: "registry.example.com/ci/runner",
		ImageTag:        "v6.0.1_27ec9940",
	})
	if err != nil {
		t.Fatalf("UpdateRunnerConfig: %v", err)
	}
	if !cfg.TokenConfigured {
		t.Fatal("an empty token must keep the stored one")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("config is not JSON: %v", err)
	}
	if values["token"] != "stored" {
		t.Fatalf("token = %v, want the stored value", values["token"])
	}
	if values["custom"] != "keep" {
		t.Fatalf("unknown key dropped: %v", values)
	}
}

func TestUpdateRunnerConfigRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		input params.UpdateProviderRunnerConfigParams
		want  string
	}{
		{
			name:  "relative base url",
			input: params.UpdateProviderRunnerConfigParams{BaseURL: "v-scheduler:38120"},
			want:  "absolute http or https",
		},
		{
			name:  "arch prefixed tag",
			input: params.UpdateProviderRunnerConfigParams{ImageTag: "amd_v6.0.1_27ec9940"},
			want:  "architecture prefix",
		},
		{
			name:  "token with space",
			input: params.UpdateProviderRunnerConfigParams{Token: "two words"},
			want:  "whitespace",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "provider.json")
			_, err := UpdateRunnerConfig(path, testCase.input)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want it to contain %q", err, testCase.want)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("a rejected update must not create the file")
			}
		})
	}
}

func TestUpdateRunnerConfigRejectsForeignJSON(t *testing.T) {
	path := writeConfigFile(t, `{"api_key":"x"}`)
	_, err := UpdateRunnerConfig(path, params.UpdateProviderRunnerConfigParams{BaseURL: "http://v-scheduler:38120/api/v1"})
	if err == nil {
		t.Fatal("a JSON config without runner keys must be rejected")
	}
}

func TestUpdateRunnerConfigRejectsNonJSON(t *testing.T) {
	path := writeConfigFile(t, "unix_socket_path = \"/var/snap/lxd/common/lxd/unix.socket\"\n")
	_, err := UpdateRunnerConfig(path, params.UpdateProviderRunnerConfigParams{BaseURL: "http://v-scheduler:38120/api/v1"})
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("error = %v, want a rejection for a non-JSON config", err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config: %v", readErr)
	}
	if string(raw) != "unix_socket_path = \"/var/snap/lxd/common/lxd/unix.socket\"\n" {
		t.Fatalf("a rejected update must not rewrite the config: %q", raw)
	}
}

func TestUpdateRunnerConfigFillsEmptyFile(t *testing.T) {
	path := writeConfigFile(t, "\n")
	cfg, err := UpdateRunnerConfig(path, params.UpdateProviderRunnerConfigParams{
		BaseURL:         "http://v-scheduler:38120/api/v1",
		ImageRepository: "registry.example.com/ci/runner",
		ImageTag:        "v6.0.1_27ec9940",
	})
	if err != nil {
		t.Fatalf("UpdateRunnerConfig: %v", err)
	}
	if !cfg.Supported || cfg.BaseURL != "http://v-scheduler:38120/api/v1" {
		t.Fatalf("unexpected answer: %+v", cfg)
	}
}
