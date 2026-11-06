package handlers

import (
	"net/netip"
	"testing"
)

func TestOIDCPublicIP(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true}, {"2001:4860:4860::8888", true}, {"127.0.0.1", false}, {"10.1.2.3", false}, {"169.254.169.254", false}, {"100.64.0.1", false}, {"::1", false}, {"fc00::1", false}, {"::ffff:127.0.0.1", false},
	} {
		if got := oidcPublicIP(netip.MustParseAddr(tc.ip)); got != tc.want {
			t.Errorf("%s: got %v", tc.ip, got)
		}
	}
}
