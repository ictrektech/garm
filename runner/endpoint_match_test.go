package runner

import (
	"testing"

	"github.com/cloudbase/garm/params"
)

func TestSameURLHost(t *testing.T) {
	cases := []struct {
		name     string
		rawURL   string
		scheme   string
		expected bool
	}{
		{
			name:     "subpath endpoint matches site root",
			rawURL:   "http://172.17.0.1:1182/app/com.ictrek.gitea/",
			scheme:   "http://172.17.0.1:1182",
			expected: true,
		},
		{
			name:     "bare host endpoint matches",
			rawURL:   "http://gitea:8080",
			scheme:   "http://gitea:8080",
			expected: true,
		},
		{
			name:     "http and https are different",
			rawURL:   "https://gitea:8080/app/com.ictrek.gitea/",
			scheme:   "http://gitea:8080",
			expected: false,
		},
		{
			name:     "different port",
			rawURL:   "http://gitea:8081",
			scheme:   "http://gitea:8080",
			expected: false,
		},
		{
			name:     "invalid url",
			rawURL:   "://nope",
			scheme:   "http://gitea:8080",
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameURLHost(tc.rawURL, tc.scheme); got != tc.expected {
				t.Fatalf("sameURLHost(%q, %q) = %v, want %v", tc.rawURL, tc.scheme, got, tc.expected)
			}
		})
	}
}

func TestPickEndpointForJob(t *testing.T) {
	internal := params.ForgeEndpoint{Name: "vos-gitea", BaseURL: "http://172.17.0.1:1182/app/com.ictrek.gitea/"}
	other := params.ForgeEndpoint{Name: "third-party", BaseURL: "https://gitea.example.com/"}

	cases := []struct {
		name      string
		endpoints []params.ForgeEndpoint
		baseURI   string
		wantName  string
		wantOK    bool
	}{
		{
			name:      "host match wins",
			endpoints: []params.ForgeEndpoint{other, internal},
			baseURI:   "http://172.17.0.1:1182",
			wantName:  "vos-gitea",
			wantOK:    true,
		},
		{
			name:      "browser host falls back to the only endpoint",
			endpoints: []params.ForgeEndpoint{internal},
			baseURI:   "https://10.144.144.232:44155",
			wantName:  "vos-gitea",
			wantOK:    true,
		},
		{
			name:      "no match with several endpoints",
			endpoints: []params.ForgeEndpoint{other, internal},
			baseURI:   "https://10.144.144.232:44155",
			wantOK:    false,
		},
		{
			name:      "no endpoints",
			endpoints: nil,
			baseURI:   "https://10.144.144.232:44155",
			wantOK:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickEndpointForJob(tc.endpoints, tc.baseURI)
			if ok != tc.wantOK {
				t.Fatalf("pickEndpointForJob() ok = %v, want %v", ok, tc.wantOK)
			}
			if got.Name != tc.wantName {
				t.Fatalf("pickEndpointForJob() name = %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}
