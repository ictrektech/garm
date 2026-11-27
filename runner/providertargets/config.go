package providertargets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cloudbase/garm/params"
)

// maxConfigSize bounds the provider config the API reads or rewrites.
const maxConfigSize = 256 * 1024

// InputError marks a rejected value, so the API answers 400 instead of 500.
type InputError struct{ msg string }

func (e *InputError) Error() string { return e.msg }

func badInput(format string, args ...any) error {
	return &InputError{msg: fmt.Sprintf(format, args...)}
}

// knownKeys are the fields the V-Scheduler provider understands. A JSON config
// without any of them belongs to a different provider, so the UI must not offer
// the runner form for it.
var knownKeys = []string{"base_url", "token", "image_tag", "image_repository", "container_port"}

// ReadRunnerConfig reports the runner-facing fields of a provider config. A
// missing file is reported as supported but empty, so the UI can create it.
func ReadRunnerConfig(path string) (params.ProviderRunnerConfig, error) {
	if path == "" {
		return params.ProviderRunnerConfig{}, nil
	}
	values, isJSON, err := readValues(path)
	if err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	// Providers with a non-JSON config, such as LXD with TOML, have no runner
	// settings. Reporting an error would warn on every page load.
	if !isJSON || !looksLikeRunnerConfig(values) {
		return params.ProviderRunnerConfig{}, nil
	}
	return params.ProviderRunnerConfig{
		Supported:       true,
		BaseURL:         stringValue(values["base_url"]),
		ImageRepository: stringValue(values["image_repository"]),
		ImageTag:        stringValue(values["image_tag"]),
		TokenConfigured: stringValue(values["token"]) != "",
	}, nil
}

// UpdateRunnerConfig validates the form values and rewrites the provider config.
// Unknown keys are preserved, so provider-specific settings survive an edit. An
// empty token keeps the stored one.
func UpdateRunnerConfig(path string, input params.UpdateProviderRunnerConfigParams) (params.ProviderRunnerConfig, error) {
	if path == "" {
		return params.ProviderRunnerConfig{}, badInput("provider has no config file")
	}
	values, isJSON, err := readValues(path)
	if err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	if !isJSON {
		return params.ProviderRunnerConfig{}, badInput("provider config is not JSON")
	}
	if !looksLikeRunnerConfig(values) {
		return params.ProviderRunnerConfig{}, badInput("provider config is not a V-Scheduler config")
	}

	baseURL, err := normalizeBaseURL(input.BaseURL)
	if err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = stringValue(values["token"])
	}
	if err := checkNoSpace("token", token, 4096); err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	repository := strings.TrimSpace(input.ImageRepository)
	if err := checkNoSpace("image_repository", repository, 1024); err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	tag := strings.TrimSpace(input.ImageTag)
	if err := checkNoSpace("image_tag", tag, 256); err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	if strings.HasPrefix(tag, "amd_") || strings.HasPrefix(tag, "arm_") {
		return params.ProviderRunnerConfig{}, badInput("image_tag %q: drop the architecture prefix, the provider adds it", tag)
	}

	values["base_url"] = baseURL
	values["token"] = token
	values["image_repository"] = repository
	values["image_tag"] = tag
	if _, ok := values["container_port"]; !ok {
		values["container_port"] = 8080
	}

	payload, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return params.ProviderRunnerConfig{}, fmt.Errorf("encoding provider config: %w", err)
	}
	payload = append(payload, '\n')
	if err := writePrivate(path, payload); err != nil {
		return params.ProviderRunnerConfig{}, err
	}
	return ReadRunnerConfig(path)
}

// readValues loads a JSON config. A missing or empty file is reported as JSON,
// so callers can create it. A config in another format, such as LXD's TOML, is
// reported as not JSON rather than as an error, so reading it stays quiet.
func readValues(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return map[string]any{}, true, nil
	case err != nil:
		return nil, false, fmt.Errorf("reading provider config: %w", err)
	case len(data) > maxConfigSize:
		return nil, false, badInput("provider config exceeds %d bytes", maxConfigSize)
	case len(bytes.TrimSpace(data)) == 0:
		return map[string]any{}, true, nil
	}
	values := map[string]any{}
	if err := json.Unmarshal(data, &values); err != nil {
		return map[string]any{}, false, nil
	}
	return values, true, nil
}

// writePrivate replaces the config atomically, so a provider invocation never
// reads a half-written file. The token lives here, so the file stays 0600.
func writePrivate(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating provider config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("creating temporary provider config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protecting temporary provider config: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temporary provider config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temporary provider config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary provider config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing provider config: %w", err)
	}
	return nil
}

func looksLikeRunnerConfig(values map[string]any) bool {
	if len(values) == 0 {
		return true
	}
	for _, key := range knownKeys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", badInput("base_url must be an absolute http or https URL")
	}
	return strings.TrimRight(value, "/"), nil
}

func checkNoSpace(field, value string, limit int) error {
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return badInput("%s must not contain whitespace", field)
	}
	if len(value) > limit {
		return badInput("%s exceeds %d characters", field, limit)
	}
	return nil
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
