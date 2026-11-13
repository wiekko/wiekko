package config

import (
	"net/netip"
	"testing"
)

func TestTrustedProxies(t *testing.T) {
	clearNexmapEnv(t)
	contains := func(cfg *Config, ip string) bool {
		addr := netip.MustParseAddr(ip)
		for _, p := range cfg.TrustedProxies {
			if p.Contains(addr) {
				return true
			}
		}
		return false
	}

	// Default: a proxy on the same host or a private network, nothing public.
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "10.1.2.3": true, "172.20.0.5": true, "192.168.1.1": true,
		"203.0.113.9": false, "172.32.0.1": false, "2001:db8::1": false,
	} {
		if got := contains(cfg, ip); got != want {
			t.Errorf("default trusts %s = %v, want %v", ip, got, want)
		}
	}

	t.Setenv("NEXMAP_TRUSTED_PROXIES", "192.0.2.7, 198.51.100.0/24")
	if cfg, err = Load(nil); err != nil {
		t.Fatalf("Load with env: %v", err)
	}
	if !contains(cfg, "192.0.2.7") || !contains(cfg, "198.51.100.200") || contains(cfg, "192.0.2.8") || contains(cfg, "127.0.0.1") {
		t.Errorf("env list not applied: %v", cfg.TrustedProxies)
	}

	if cfg, err = Load([]string{"--trusted-proxies", "none"}); err != nil {
		t.Fatalf("Load with flag: %v", err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("none still trusts %v", cfg.TrustedProxies)
	}

	for _, bad := range []string{"proxy.example.com", "10.0.0.0/33", "10.0.0.1,nope"} {
		t.Setenv("NEXMAP_TRUSTED_PROXIES", bad)
		if _, err := Load(nil); err == nil {
			t.Errorf("Load accepted trusted proxies %q", bad)
		}
	}
}
