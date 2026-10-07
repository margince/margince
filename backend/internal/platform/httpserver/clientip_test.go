// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// What these prove: X-Forwarded-For moves the throttle key only when a trusted
// peer delivered it, and then only as far left as that peer's own network
// vouches for. The spoofing cases are the point — a key the sender can choose
// per request is no key at all, and it is strictly worse than the proxy-keyed
// default this middleware improves on.

// clientIPThrough runs one request through ResolveClientIP and reports the key
// a limiter behind it would have used.
func clientIPThrough(t *testing.T, trusted TrustedProxies, remoteAddr string, xff ...string) string {
	t.Helper()
	var got string
	h := ResolveClientIP(trusted, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	r.RemoteAddr = remoteAddr
	for _, line := range xff {
		r.Header.Add("X-Forwarded-For", line)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	return got
}

func mustTrust(t *testing.T, raw string) TrustedProxies {
	t.Helper()
	trusted, err := ParseTrustedProxies(raw)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%q): %v", raw, err)
	}
	return trusted
}

func TestClientIP_NoTrustedProxiesKeysOnThePeer(t *testing.T) {
	got := clientIPThrough(t, TrustedProxies{}, "10.0.115.44:51234", "203.0.113.7")
	if got != "10.0.115.44" {
		t.Fatalf("with nobody trusted the key is the TCP peer; got %q", got)
	}
}

func TestClientIP_SpoofedHeaderFromUntrustedPeerIsIgnored(t *testing.T) {
	trusted := mustTrust(t, "10.0.96.0/19")
	// A client reaching the pod directly, claiming to be someone else.
	got := clientIPThrough(t, trusted, "198.51.100.9:40000", "203.0.113.7")
	if got != "198.51.100.9" {
		t.Fatalf("an untrusted peer's X-Forwarded-For must not move the key; got %q", got)
	}
}

func TestClientIP_TrustedPeerIsHonoured(t *testing.T) {
	trusted := mustTrust(t, "10.0.96.0/19")
	got := clientIPThrough(t, trusted, "10.0.115.44:51234", "203.0.113.7")
	if got != "203.0.113.7" {
		t.Fatalf("a trusted proxy's X-Forwarded-For names the client; got %q", got)
	}
}

func TestClientIP_MultiHopTakesTheRightmostUntrustedEntry(t *testing.T) {
	// The D13 shape: load balancer (public subnet) -> ingress controller
	// (private subnet) -> api. The client prepended a forged entry of their own;
	// the load balancer appended the real client, the controller appended the
	// load balancer.
	trusted := mustTrust(t, "10.0.0.0/19, 10.0.96.0/19")
	got := clientIPThrough(t, trusted, "10.0.115.44:51234", "192.0.2.66, 203.0.113.7, 10.0.12.34")
	if got != "203.0.113.7" {
		t.Fatalf("want the first untrusted hop from the right, not the forged leftmost; got %q", got)
	}
}

func TestClientIP_HopsAcrossSeveralHeaderLinesAreOneChain(t *testing.T) {
	trusted := mustTrust(t, "10.0.0.0/16")
	got := clientIPThrough(t, trusted, "10.0.115.44:51234", "192.0.2.66, 203.0.113.7", "10.0.12.34")
	if got != "203.0.113.7" {
		t.Fatalf("a second header line continues the chain; got %q", got)
	}
}

func TestClientIP_EveryHopTrustedAnswersTheLeftmost(t *testing.T) {
	// A caller inside the cluster, through the ingress.
	trusted := mustTrust(t, "10.0.0.0/16")
	got := clientIPThrough(t, trusted, "10.0.115.44:51234", "10.0.107.29")
	if got != "10.0.107.29" {
		t.Fatalf("an all-trusted chain begins at its leftmost hop; got %q", got)
	}
}

func TestClientIP_MalformedHopFallsBackToThePeer(t *testing.T) {
	trusted := mustTrust(t, "10.0.0.0/16")
	for _, xff := range []string{"203.0.113.7, not-an-ip", "203.0.113.7,,10.0.12.34", "203.0.113.7:443"} {
		if got := clientIPThrough(t, trusted, "10.0.115.44:51234", xff); got != "10.0.115.44" {
			t.Errorf("X-Forwarded-For %q: a broken chain must key on the peer, never on a guess; got %q", xff, got)
		}
	}
}

func TestClientIP_TrustedPeerWithoutHeaderKeysOnThePeer(t *testing.T) {
	trusted := mustTrust(t, "10.0.0.0/16")
	if got := clientIPThrough(t, trusted, "10.0.115.44:51234"); got != "10.0.115.44" {
		t.Fatalf("no header, nothing to believe; got %q", got)
	}
}

func TestClientIP_MappedAddressesCompareAsIPv4(t *testing.T) {
	trusted := mustTrust(t, "10.0.0.0/16")
	got := clientIPThrough(t, trusted, "[::ffff:10.0.115.44]:51234", "::ffff:203.0.113.7")
	if got != "203.0.113.7" {
		t.Fatalf("a v4-mapped peer is the same peer; got %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	if trusted := mustTrust(t, ""); !trusted.Empty() {
		t.Fatal("empty configures nobody")
	}
	if got := mustTrust(t, " 10.0.96.0/19 ,10.0.1.1, 10.0.128.7/19").String(); got != "10.0.96.0/19,10.0.1.1/32,10.0.128.0/19" {
		t.Fatalf("bare address is its own /32 and a prefix is masked; got %q", got)
	}
	if got := mustTrust(t, "::ffff:10.0.0.0/112").String(); got != "10.0.0.0/16" {
		t.Fatalf("a v4-mapped prefix is rebased onto IPv4, or unmapped peers never match it; got %q", got)
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "::ffff:0:0/80", "::ffff:0.0.0.0/96", "10.0.0.0/33", "ingress", "10.0.0.0/16,nope"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("ParseTrustedProxies(%q) must refuse", bad)
		}
	}
}
