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
// describe a provider to the pool form. Providers with a different config format
// have none of these keys and yield an empty answer, which makes the UI hide the
// picker.
//
// The provider itself reads the same /managed-nodes answer with its own struct
// (contrib/garm-provider-vscheduler/vscheduler.go, type node). Change both when
// that shape changes.
type vschedulerConfig struct {
	BaseURL         string `json:"base_url"`
	Token           string `json:"token"`
	Timeout         int    `json:"timeout_seconds"`
	ImageRepository string `json:"image_repository"`
	ImageTag        string `json:"image_tag"`
}

// List describes the deployment targets of an external provider, plus the runner
// image repository and tag it expects. A missing config file or a provider with a
// different config format yields an empty answer and no error.
func List(ctx context.Context, configFile string) (params.ProviderTargetInfo, error) {
	if configFile == "" {
		return params.ProviderTargetInfo{}, nil
	}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return params.ProviderTargetInfo{}, nil
		}
		return params.ProviderTargetInfo{}, fmt.Errorf("reading provider config: %w", err)
	}
	var cfg vschedulerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		// Providers with a non-JSON config (for example the LXD provider's TOML)
		// simply have no target list. Reporting that as an error would put a
		// warning in the log every time someone opens the pool form.
		return params.ProviderTargetInfo{}, nil
	}

	info := params.ProviderTargetInfo{
		Targets:         params.ProviderTargets{},
		ImageRepository: cfg.ImageRepository,
		ImageTag:        cfg.ImageTag,
	}
	if cfg.BaseURL == "" || cfg.Token == "" {
		return info, nil
	}

	targets, err := listVschedulerNodes(ctx, cfg)
	if err != nil {
		// Keep the image fields: they are useful even when the node list is not
		// reachable. The controller logs the error and answers with empty targets.
		return info, err
	}
	info.Targets = targets
	return info, nil
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
