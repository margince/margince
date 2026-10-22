// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// TrustedProxies is the set of direct peers whose X-Forwarded-For this process
// believes. The zero value trusts nobody, which is the posture of a process
// reached directly: every throttle key is then the TCP peer, exactly as before
// the set existed.
//
// It exists because a deployment fronted by a reverse proxy otherwise keys
// every per-IP limit on the PROXY. Behind one ingress controller that is one
// bucket for the whole internet — thirty sign-in attempts a minute shared by
// every user, which one abuser spends for all of them — and behind N
// controllers it is N buckets that each hold a random slice of everybody.
type TrustedProxies struct {
	prefixes []netip.Prefix
}

// ParseTrustedProxies reads a comma-separated list of CIDR prefixes or bare
// addresses (a bare address is its own /32 or /128). Empty is the zero value.
//
// A prefix of length zero is REFUSED rather than honoured: trusting every
// address makes every hop in X-Forwarded-For believable, so the resolved client
// is the leftmost entry — the one the sender wrote — and the throttle is then
// keyed on a value an attacker picks per request. That is strictly worse than
// the proxy-keyed default this setting exists to improve on, so it is a boot
// error and never a silent reading.
func ParseTrustedProxies(raw string) (TrustedProxies, error) {
	var out TrustedProxies
	for field := range strings.SplitSeq(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		prefix, err := parseTrustedEntry(field)
		if err != nil {
			return TrustedProxies{}, err
		}
		if prefix.Bits() == 0 {
			return TrustedProxies{}, fmt.Errorf("trusted proxy %q trusts every address, which makes X-Forwarded-For "+
				"attacker-chosen: name the proxies' own network instead", field)
		}
		out.prefixes = append(out.prefixes, prefix)
	}
	return out, nil
}

func parseTrustedEntry(field string) (netip.Prefix, error) {
	if strings.Contains(field, "/") {
		prefix, err := netip.ParsePrefix(field)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("trusted proxy %q is not a CIDR prefix: %w", field, err)
		}
		// Addresses are compared unmapped, so a v4-mapped prefix is rebased onto
		// IPv4 or it would never match anything. One shorter than the mapping's
		// own 96 bits reaches outside ::ffff:0:0/96 and names no IPv4 network.
		if prefix.Addr().Is4In6() {
			if prefix.Bits() < 96 {
				return netip.Prefix{}, fmt.Errorf("trusted proxy %q is a v4-mapped prefix shorter than /96: "+
					"write the IPv4 network instead", field)
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(field)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("trusted proxy %q is neither an address nor a CIDR prefix: %w", field, err)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// Empty reports whether the set trusts nobody.
func (t TrustedProxies) Empty() bool { return len(t.prefixes) == 0 }

// String is the configured set as it was understood, for the boot log line.
func (t TrustedProxies) String() string {
	parts := make([]string, len(t.prefixes))
	for i, p := range t.prefixes {
		parts[i] = p.String()
	}
	return strings.Join(parts, ",")
}

func (t TrustedProxies) contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range t.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

type clientIPKey struct{}

// ResolveClientIP decides, once per request, the address every per-IP throttle
// in this process keys on, and carries it to ClientIP on the request context.
//
// X-Forwarded-For is read ONLY when the direct peer is a trusted proxy, and
// then from the RIGHT: each hop appends the address it received the request
// from, so the entries a trusted proxy wrote are the rightmost ones and
// everything to their left is whatever the previous sender chose. The client
// is the first entry, walking leftwards, that is not itself a trusted proxy.
// An attacker may prepend as many entries as they like; none of them is ever
// reached, because the untrusted address their own connection arrived from was
// appended to the right of them.
//
// An entry that does not parse ends the walk at the PEER rather than at a
// guess. Only a trusted hop can have written anything to the right of the real
// client, so a malformed entry there is a proxy misconfiguration, and the
// proxy-keyed answer is the one that cannot be steered.
//
// r.RemoteAddr is left as it is: it is still the TCP peer, and code that asks
// for the peer should get the peer.
func ResolveClientIP(trusted TrustedProxies, next http.Handler) http.Handler {
	if trusted.Empty() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), clientIPKey{}, trusted.resolve(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromTrustedPeer reports whether the request's direct peer is a trusted
// proxy, the only sender whose X-Forwarded-* headers this process believes.
func (t TrustedProxies) FromTrustedPeer(r *http.Request) bool {
	addr, err := netip.ParseAddr(peerHost(r))
	return err == nil && t.contains(addr)
}

func (t TrustedProxies) resolve(r *http.Request) string {
	peer := peerHost(r)
	if !t.FromTrustedPeer(r) {
		return peer
	}
	// Every X-Forwarded-For line, in order: a proxy that adds a second header
	// line rather than extending the first is saying the same thing, and
	// reading only the first line would stop the walk short of the client.
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(line, ",")...)
	}
	client := peer
	for _, hop := range slices.Backward(hops) {
		hop, err := netip.ParseAddr(strings.TrimSpace(hop))
		if err != nil {
			return peer
		}
		client = hop.Unmap().String()
		if !t.contains(hop) {
			return client
		}
	}
	// Every hop was a trusted proxy: the request began inside the trusted
	// network, and its origin is the leftmost hop a proxy recorded.
	return client
}

// ClientIP is the ONE client-IP throttle key in this process — the login and
// password-reset limits in identity, the anonymous booking and preference
// paths, and every connector edge. It is here rather than in either caller
// because two copies meant a deployment could harden one edge and leave the
// other keyed differently, and nothing would say so.
//
// It is the address ResolveClientIP decided, and the direct peer wherever that
// middleware did not run or trusts nobody. A raw X-Forwarded-For is
// attacker-chosen and is never read here: only a header a TRUSTED peer
// delivered is believed, and only the hops that peer vouches for.
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerHost(r)
}

func peerHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
