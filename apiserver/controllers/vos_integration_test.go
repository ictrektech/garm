package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVOSBrowserOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://vos.example.com:1180/app/com.ictrek.garm/api/v1/auth/vos", nil)
	for origin, expected := range map[string]string{
		"https://vos.example.com:1180":           "https://vos.example.com:1180",
		"https://other.example.com":              "",
		"https://vos.example.com:1180/some/path": "",
		"http://vos.example.com:1180":            "",
		"":                                       "",
	} {
		req.Header.Set("Origin", origin)
		require.Equal(t, expected, vosBrowserOrigin(req), origin)
	}
}

func TestVOSOptionalGiteaProbe(t *testing.T) {
	for _, check := range []struct {
		code int
		body string
		want bool
	}{
		{http.StatusOK, "OK", true},
		{http.StatusOK, "<html>VOS</html>", false},
		{http.StatusNotFound, "Not Found", false},
		{http.StatusServiceUnavailable, "Starting", false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/healthz", r.URL.Path)
			w.WriteHeader(check.code)
			_, _ = w.Write([]byte(check.body))
		}))
		require.Equal(t, check.want, vosGiteaAvailable(context.Background(), server.URL))
		server.Close()
	}
}
