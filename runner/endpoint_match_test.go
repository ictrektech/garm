package runner

import "testing"

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
