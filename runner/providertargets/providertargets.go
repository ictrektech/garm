// Package providertargets lists the compute targets an external provider can
// deploy to, so the web UI can offer a picker instead of hand-written
// extra_specs.
package providertargets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudbase/garm/params"
)

const defaultTimeout = 10 * time.Second

// vschedulerConfig is the subset of the V-Scheduler provider config needed to
// list nodes. Providers with a different config format have neither key and
// yield an empty list, which makes the UI hide the picker.
//
// The provider itself reads the same /managed-nodes answer with its own struct
// (contrib/garm-provider-vscheduler/vscheduler.go, type node). Change both when
// that shape changes.
type vschedulerConfig struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
	Timeout int    `json:"timeout_seconds"`
}

// List returns the deployment targets described by an external provider config
// file. An empty result and a nil error mean the provider has no target list to
// offer.
func List(ctx context.Context, configFile string) (params.ProviderTargets, error) {
	if configFile == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading provider config: %w", err)
	}
	var cfg vschedulerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing provider config: %w", err)
	}
	if cfg.BaseURL == "" || cfg.Token == "" {
		return nil, nil
	}
	return listVschedulerNodes(ctx, cfg)
}

func listVschedulerNodes(ctx context.Context, cfg vschedulerConfig) (params.ProviderTargets, error) {
	timeout := defaultTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/managed-nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing managed nodes: %s returned %d", url, resp.StatusCode)
	}
	var data struct {
		Data struct {
			Items params.ProviderTargets `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding managed nodes: %w", err)
	}
	return data.Data.Items, nil
}
