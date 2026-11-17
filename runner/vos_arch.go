package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	commonParams "github.com/cloudbase/garm-provider-common/params"

	"github.com/cloudbase/garm/auth"
)

const (
	// vosSystemInfoPath is the VOS endpoint that reports the device caps,
	// including the architecture of the VOS device.
	vosSystemInfoPath = "/v1000/system/info"

	// DefaultVOSSystemInfoURL is the VOS system info endpoint reachable from
	// applications running on the VOS host network.
	DefaultVOSSystemInfoURL = "http://172.17.0.1:8105" + vosSystemInfoPath

	vosSystemInfoTimeout = 2 * time.Second

	// A VOS backend that is still booting refuses connections instead of timing
	// out, so one quick retry covers the startup race between the two services.
	vosArchAttempts   = 2
	vosArchRetryDelay = 300 * time.Millisecond

	maxVOSSystemInfoSize = 64 << 10
)

// errUnsupportedVOSArch reports a value VOS explicitly returned but GARM has no
// target for. It is not a transient failure: guessing an architecture would
// create a pool whose instances cannot run a runner.
var errUnsupportedVOSArch = errors.New("unsupported VOS device architecture")

// vosSystemInfoResponse is the subset of GET /v1000/system/info GARM reads.
// VOS wraps successful payloads in {code, category, data, msg, version} and
// reports business errors as HTTP 200 with a non zero code and a null data.
type vosSystemInfoResponse struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg"`
	Data *vosSystemInfoData `json:"data"`
}

// vosSystemInfoData holds the device caps GARM reads. Caps metadata values are
// not always strings (a deployment may declare
// "skip-license-verification: true"), so the map stays untyped and only arch is
// read as a string.
type vosSystemInfoData struct {
	Caps struct {
		Metadata map[string]any `json:"metadata"`
	} `json:"caps"`
}

// The architecture of the device does not change while GARM runs, so one
// successful read is enough. Failed reads are not cached: VOS may simply not be
// reachable yet, and the next ensure call should try again.
var (
	vosArchMutex sync.Mutex
	vosArchValue commonParams.OSArch
)

// vosSystemInfoURL returns the VOS system info endpoint. A deployment only has
// to configure GARM_VOS_USERINFO_URL: the system info endpoint lives on the same
// host and port.
func vosSystemInfoURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("GARM_VOS_SYSTEM_INFO_URL")); endpoint != "" {
		return endpoint
	}

	userinfo := strings.TrimSpace(os.Getenv("GARM_VOS_USERINFO_URL"))
	if userinfo == "" {
		userinfo = auth.DefaultVOSUserInfoURL
	}
	return replaceURLPath(userinfo, vosSystemInfoPath)
}

// replaceURLPath swaps the path of an absolute URL, keeping scheme, host and
// port. Anything GARM cannot parse falls back to the default endpoint.
func replaceURLPath(rawURL, path string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return DefaultVOSSystemInfoURL
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// parseVOSArch maps the VOS caps arch value to a GARM OSArch. GARM only offers
// amd64 and arm64 targets, so 32-bit values are not accepted.
func parseVOSArch(value string) (commonParams.OSArch, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "arm64", "aarch64":
		return commonParams.Arm64, true
	case "amd64", "x86_64":
		return commonParams.Amd64, true
	default:
		return "", false
	}
}

// hostArch maps the architecture the controller was built for to an OSArch.
func hostArch() commonParams.OSArch {
	if arch, ok := parseVOSArch(runtime.GOARCH); ok {
		return arch
	}
	return commonParams.Amd64
}

// fetchVOSArch reads the architecture of the VOS device from the system info
// endpoint.
func fetchVOSArch(ctx context.Context, endpoint string) (commonParams.OSArch, error) {
	reqCtx, cancel := context.WithTimeout(ctx, vosSystemInfoTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("building VOS system info request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling VOS system info at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("VOS system info at %s returned %s", endpoint, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVOSSystemInfoSize))
	if err != nil {
		return "", fmt.Errorf("reading VOS system info: %w", err)
	}

	var info vosSystemInfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("decoding VOS system info: %w", err)
	}

	if info.Code != 0 {
		return "", fmt.Errorf("VOS system info returned code %d: %s", info.Code, strings.TrimSpace(info.Msg))
	}
	if info.Data == nil {
		return "", fmt.Errorf("VOS system info returned no data: %s", strings.TrimSpace(info.Msg))
	}

	raw, ok := info.Data.Caps.Metadata["arch"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("VOS system info does not report a device architecture (caps metadata: %d keys)",
			len(info.Data.Caps.Metadata))
	}

	arch, ok := parseVOSArch(raw)
	if !ok {
		return "", fmt.Errorf("%w: %q", errUnsupportedVOSArch, raw)
	}
	return arch, nil
}

// vosArch returns the architecture of the VOS device.
//
// An unreachable VOS falls back to the architecture of the controller process:
// a registration must not fail because the VOS backend was slow to start. A
// value VOS explicitly reports but GARM cannot use is returned as an error
// instead, because the pool it would create can never run a runner and is not
// corrected later.
func (r *Runner) vosArch(ctx context.Context) (commonParams.OSArch, error) {
	vosArchMutex.Lock()
	cached := vosArchValue
	vosArchMutex.Unlock()
	if cached != "" {
		return cached, nil
	}

	endpoint := vosSystemInfoURL()
	var lastErr error
	for attempt := 1; attempt <= vosArchAttempts; attempt++ {
		arch, err := fetchVOSArch(ctx, endpoint)
		if err == nil {
			vosArchMutex.Lock()
			vosArchValue = arch
			vosArchMutex.Unlock()
			return arch, nil
		}

		if errors.Is(err, errUnsupportedVOSArch) {
			return "", fmt.Errorf("resolving the VOS device architecture: %w", err)
		}
		lastErr = err

		if attempt < vosArchAttempts {
			slog.WarnContext(ctx, "reading the VOS device architecture failed, retrying",
				"error", err, "attempt", attempt)
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("resolving the VOS device architecture: %w", ctx.Err())
			case <-time.After(vosArchRetryDelay):
			}
		}
	}

	slog.WarnContext(ctx, "could not read the VOS device architecture, falling back to the controller architecture",
		"error", lastErr, "arch", hostArch())
	return hostArch(), nil
}
