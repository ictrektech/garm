package runner

import (
	"context"
	"testing"

	"github.com/cloudbase/garm/config"
	"github.com/cloudbase/garm/params"
)

func TestVOSPoolParams(t *testing.T) {
	r := &Runner{ctx: context.Background(), config: config.Config{Providers: []config.Provider{{Name: "lxd"}}}}

	got := r.vosPoolParams(params.EnsureEntityParams{Labels: []string{"garm-pools"}})
	if got.ProviderName != "lxd" {
		t.Fatalf("ProviderName = %q, want lxd", got.ProviderName)
	}
	if got.MaxRunners != 2 {
		t.Fatalf("MaxRunners = %d, want the default 2", got.MaxRunners)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "garm-pools" {
		t.Fatalf("Tags = %v, want [garm-pools]", got.Tags)
	}

	got = r.vosPoolParams(params.EnsureEntityParams{Labels: []string{"linux"}, MaxRunners: 3})
	if got.MaxRunners != 3 {
		t.Fatalf("MaxRunners = %d, want 3", got.MaxRunners)
	}

	r.config.Providers = nil
	if got := r.vosPoolParams(params.EnsureEntityParams{}); got.ProviderName != "lxd" {
		t.Fatalf("ProviderName = %q, want the lxd fallback", got.ProviderName)
	}
}

func TestNewWebhookSecret(t *testing.T) {
	first, second := newWebhookSecret(), newWebhookSecret()
	if len(first) != 64 {
		t.Fatalf("secret length = %d, want 64", len(first))
	}
	if first == second {
		t.Fatalf("secrets are not unique")
	}
}
