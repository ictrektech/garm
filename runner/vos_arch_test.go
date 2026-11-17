package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	commonParams "github.com/cloudbase/garm-provider-common/params"
)

// resetVOSArchCache clears the cached architecture between tests.
func resetVOSArchCache(t *testing.T) {
	t.Helper()
	vosArchMutex.Lock()
	defer vosArchMutex.Unlock()
	vosArchValue = ""
}

// vosSystemInfoHandler serves a VOS system info payload carrying arch. The
// payload mirrors a real device response, which mixes string and boolean
// metadata values (skip-license-verification is a bool).
func vosSystemInfoHandler(arch string) http.Handler {
	return rawHandler(`{"code":0,"category":"ok","data":{"vendor":"vivibit","device_type":"generic",` +
		`"caps":{"metadata":{"arch":"` + arch + `","device-type":"generic",` +
		`"skip-license-verification":true,"vendor-id":"vivibit"},` +
		`"hardware":{},"software":{"cuda":{"enabled":true,"extra":{}}}},"device_name":null,` +
		`"device_id":"67f916"},"msg":"","version":"1.1.0"}`)
}

func rawHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func TestParseVOSArch(t *testing.T) {
	cases := []struct {
		value string
		want  commonParams.OSArch
		ok    bool
	}{
		{value: "arm64", want: commonParams.Arm64, ok: true},
		{value: "ARM64", want: commonParams.Arm64, ok: true},
		{value: " aarch64 ", want: commonParams.Arm64, ok: true},
		{value: "amd64", want: commonParams.Amd64, ok: true},
		{value: "x86_64", want: commonParams.Amd64, ok: true},
		{value: "386", ok: false},
		{value: "", ok: false},
		{value: "riscv64", ok: false},
	}

	for _, tc := range cases {
		got, ok := parseVOSArch(tc.value)
		if ok != tc.ok {
			t.Fatalf("parseVOSArch(%q) ok = %v, want %v", tc.value, ok, tc.ok)
		}
		if got != tc.want {
			t.Fatalf("parseVOSArch(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestReplaceURLPath(t *testing.T) {
	got := replaceURLPath("http://172.17.0.1:1182/v1000/oauth2/userinfo", vosSystemInfoPath)
	if want := "http://172.17.0.1:1182/v1000/system/info"; got != want {
		t.Fatalf("replaceURLPath() = %q, want %q", got, want)
	}

	if got := replaceURLPath("not a url", vosSystemInfoPath); got != DefaultVOSSystemInfoURL {
		t.Fatalf("replaceURLPath() = %q, want the default endpoint", got)
	}
}

func TestVOSSystemInfoURL(t *testing.T) {
	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", "")
	t.Setenv("GARM_VOS_USERINFO_URL", "http://192.168.1.2:1180/v1000/oauth2/userinfo")
	if got := vosSystemInfoURL(); got != "http://192.168.1.2:1180/v1000/system/info" {
		t.Fatalf("vosSystemInfoURL() = %q, want the derived system info endpoint", got)
	}

	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", "http://10.0.0.5:9999/v1000/system/info")
	if got := vosSystemInfoURL(); got != "http://10.0.0.5:9999/v1000/system/info" {
		t.Fatalf("vosSystemInfoURL() = %q, want the explicit override", got)
	}
}

func TestFetchVOSArch(t *testing.T) {
	server := httptest.NewServer(vosSystemInfoHandler("arm64"))
	defer server.Close()

	arch, err := fetchVOSArch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchVOSArch() error = %v", err)
	}
	if arch != commonParams.Arm64 {
		t.Fatalf("fetchVOSArch() = %q, want arm64", arch)
	}
}

func TestFetchVOSArchRejectsUnsupportedValues(t *testing.T) {
	for _, payload := range []string{"386", "riscv64"} {
		server := httptest.NewServer(vosSystemInfoHandler(payload))
		_, err := fetchVOSArch(context.Background(), server.URL)
		server.Close()

		if !errors.Is(err, errUnsupportedVOSArch) {
			t.Fatalf("fetchVOSArch() with arch %q error = %v, want errUnsupportedVOSArch", payload, err)
		}
	}
}

func TestFetchVOSArchMissingArch(t *testing.T) {
	// A device that reports caps without an arch is a broken payload, not an
	// architecture GARM refuses to run.
	server := httptest.NewServer(rawHandler(
		`{"code":0,"data":{"caps":{"metadata":{"device-type":"generic"}}},"msg":"","version":"1.1.0"}`))
	defer server.Close()

	_, err := fetchVOSArch(context.Background(), server.URL)
	if err == nil {
		t.Fatalf("fetchVOSArch() returned no error for a payload without arch")
	}
	if errors.Is(err, errUnsupportedVOSArch) {
		t.Fatalf("fetchVOSArch() reported a missing arch as unsupported: %v", err)
	}
}

func TestFetchVOSArchReportsVOSErrors(t *testing.T) {
	// VOS reports business errors as HTTP 200 with a non zero code and a null
	// data section.
	payloads := map[string]string{
		"business error": `{"code":10017,"category":"vos-platform","data":null,"msg":"internal error","version":"1.1.0"}`,
		"no code":        `{"data":null,"msg":"","version":"1.1.0"}`,
	}

	for name, payload := range payloads {
		server := httptest.NewServer(rawHandler(payload))
		_, err := fetchVOSArch(context.Background(), server.URL)
		server.Close()

		if err == nil {
			t.Fatalf("fetchVOSArch() with %s returned no error", name)
		}
		if errors.Is(err, errUnsupportedVOSArch) {
			t.Fatalf("fetchVOSArch() with %s reported it as an unsupported architecture: %v", name, err)
		}
	}
}

func TestFetchVOSArchHandlesHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := fetchVOSArch(context.Background(), server.URL); err == nil {
		t.Fatalf("fetchVOSArch() returned no error for a 500 response")
	}
}

func TestVOSArchCachesSuccessfulLookupOnly(t *testing.T) {
	resetVOSArchCache(t)

	var requests atomic.Int32
	var arch atomic.Value
	arch.Store("arm64")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		vosSystemInfoHandler(arch.Load().(string)).ServeHTTP(w, r)
	}))
	defer server.Close()

	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", server.URL)
	r := &Runner{ctx: context.Background()}

	got, err := r.vosArch(context.Background())
	if err != nil || got != commonParams.Arm64 {
		t.Fatalf("vosArch() = (%q, %v), want (arm64, nil)", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}

	// A second call must come from the cache, even if VOS would answer
	// differently now.
	arch.Store("amd64")
	got, err = r.vosArch(context.Background())
	if err != nil || got != commonParams.Arm64 {
		t.Fatalf("vosArch() = (%q, %v), want the cached arm64", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want the cache to avoid a second call", got)
	}
}

func TestVOSArchRetriesTransientFailure(t *testing.T) {
	resetVOSArchCache(t)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		vosSystemInfoHandler("arm64").ServeHTTP(w, r)
	}))
	defer server.Close()

	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", server.URL)
	r := &Runner{ctx: context.Background()}

	got, err := r.vosArch(context.Background())
	if err != nil || got != commonParams.Arm64 {
		t.Fatalf("vosArch() = (%q, %v), want (arm64, nil) after a retry", got, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestVOSArchReturnsErrorForUnsupportedValue(t *testing.T) {
	resetVOSArchCache(t)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		vosSystemInfoHandler("386").ServeHTTP(w, r)
	}))
	defer server.Close()

	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", server.URL)
	r := &Runner{ctx: context.Background()}

	// A value GARM cannot use must not be replaced by a guess: the caller has
	// to stop instead of creating a pool that cannot run runners.
	if _, err := r.vosArch(context.Background()); !errors.Is(err, errUnsupportedVOSArch) {
		t.Fatalf("vosArch() error = %v, want errUnsupportedVOSArch", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want an unsupported value not to be retried", got)
	}
}

func TestVOSArchDoesNotCacheFallback(t *testing.T) {
	resetVOSArchCache(t)

	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		vosSystemInfoHandler("arm64").ServeHTTP(w, r)
	}))
	defer server.Close()

	t.Setenv("GARM_VOS_SYSTEM_INFO_URL", server.URL)
	r := &Runner{ctx: context.Background()}

	got, err := r.vosArch(context.Background())
	if err != nil || got != hostArch() {
		t.Fatalf("vosArch() = (%q, %v), want the host architecture fallback", got, err)
	}

	// VOS becoming reachable later must be picked up rather than the fallback
	// being pinned for the lifetime of the process.
	healthy.Store(true)
	got, err = r.vosArch(context.Background())
	if err != nil || got != commonParams.Arm64 {
		t.Fatalf("vosArch() = (%q, %v), want (arm64, nil) once VOS answers", got, err)
	}
}
