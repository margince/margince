// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where a binding's base_url is allowed to take the server.
//
// A binding is operator-supplied, and `ollama`, `vllm` and `openai_compatible`
// take a host with nothing constraining it — so without a guard the stored
// routing document is a request the server makes on whoever can write it, at
// whatever address they name. That is an SSRF sink whether or not the writer
// was trusted to bind a model, and the answer comes back to them: the
// available-models read reflects the decoded body.
//
// The guard cannot simply be netguard.RefusePrivate, which is what every other
// tenant-host lane in this tree uses, because a private address is the FEATURE
// here — a local Ollama, a GPU box on the operator's own network, a self-hosted
// gateway. So the allowance is derived from the binding instead: what a lane
// may dial depends on what that lane is for.

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"

	"github.com/margince/margince/backend/internal/platform/netguard"
)

// egressClass is how far one provider's binding may reach.
type egressClass int

const (
	// egressPublicOnly is the vendor lane: `anthropic`, `openai` and `gemini`
	// call a named vendor's public API, and their base_url is documented as an
	// override of that vendor's host — never as a way into the deployment's own
	// network. It is also the lane that carries the installation's BYOK key in
	// a header Go does not strip across hosts (x-api-key, x-goog-api-key), so
	// an address here is a credential's destination as much as a request's.
	//
	// The ZERO value on purpose: a provider added without a considered entry in
	// providerEgress gets the strict lane rather than the permissive one.
	egressPublicOnly egressClass = iota

	// egressOperatorEndpoint is the lane an operator points at their own
	// infrastructure. Loopback and the private ranges are allowed because they
	// are what this lane exists for; a public host is allowed because a hosted
	// Ollama or a broker on the OpenAI wire is an ordinary binding. What is
	// refused is everything in between — the reserved ranges that are nobody's
	// inference endpoint and are how an internal address gets named without
	// looking like one, link-local cloud metadata first among them.
	egressOperatorEndpoint
)

// providerEgress is the one place a provider's reach is decided, read by both
// the dialer and the write-time rule so they cannot answer differently.
// TestEveryProviderDeclaresAnEgressClass holds it complete.
var providerEgress = map[string]egressClass{
	// The fake adapter opens no socket, so its class is inert; it is listed
	// because a provider missing from this map is the thing the census catches.
	ProviderFake:      egressPublicOnly,
	providerAnthropic: egressPublicOnly,
	providerOpenAI:    egressPublicOnly,
	providerGemini:    egressPublicOnly,
	providerOllama:    egressOperatorEndpoint,
	providerVLLM:      egressOperatorEndpoint,
	// The one BYOK provider on the operator lane, and the only entry here that
	// does not follow from ProviderIsLocal. `openai_compatible` is the adapter
	// for "any vendor on the OpenAI wire", and a self-hosted gateway on the
	// operator's own network is a documented one — so a private address is a
	// binding this lane must serve, and the key travelling there travels to the
	// operator's own infrastructure. TestEveryLocalProviderTakesTheOperatorLane
	// names it as the sole exception, so a future adapter cannot join it quietly.
	providerOpenAICompatible: egressOperatorEndpoint,
}

// egressFor answers for a provider name that may not be one this build knows —
// SelectBrain refuses those, but the write-time rule runs before it, and the
// missing entry must not read as the permissive class.
func egressFor(provider string) egressClass { return providerEgress[provider] }

// addressAllowed reports whether class may dial the concrete address ip.
//
// Held by: TestTheWriteRuleAndTheDialerAgree (backend/internal/modules/ai/outboundegress_test.go)
//
// The single spelling of the rule: the dial-time Control hook and the
// write-time base_url check both ask it, so a binding accepted at the door
// cannot be one the first call refuses.
func addressAllowed(class egressClass, ip net.IP) bool {
	if class == egressOperatorEndpoint && customerControlled(ip) {
		return true
	}
	return netguard.PublicIP(ip)
}

// dialGuard is the net.Dialer.Control hook for one egress class. It runs after
// DNS on the address the socket is about to connect to, so a name that resolves
// — or rebinds — to a refused address is stopped at connect time rather than
// pre-checked and then dialed anyway.
func dialGuard(class egressClass) func(string, string, syscall.RawConn) error {
	return func(network, address string, conn syscall.RawConn) error {
		host, port, err := net.SplitHostPort(address)
		if err == nil {
			// parseHostAddress, not net.ParseIP: a link-local or unique-local
			// address arrives here carrying its zone ("fe80::1%eth0"), which
			// ParseIP does not take — and the write-time rule already drops it.
			// A second reading of the same address is how the two ends of one
			// rule start disagreeing.
			if ip := parseHostAddress(host); ip != nil {
				if addressAllowed(class, ip) {
					return nil
				}
				// The zone stripped, so netguard names the address it judged
				// rather than reporting a zoned one as "not a literal IP".
				address = net.JoinHostPort(ip.String(), port)
			}
		}
		// Refused, or a shape this cannot judge for itself. netguard owns both
		// verdicts and names which one in its message, so there is no second
		// wording of "we will not dial that" to keep in step with it.
		return netguard.RefusePrivate(network, address, conn)
	}
}

// parseHostAddress is the IP a url host names, or nil when it names a DNS name.
//
// A zone ("fe80::1%eth0") says which interface an address is reached on and
// net.ParseIP does not take one; it is dropped because the judgement is about
// the address, and an interface cannot change what an address is.
func parseHostAddress(host string) net.IP {
	address, _, _ := strings.Cut(host, "%")
	return net.ParseIP(address)
}

// customerControlled reports whether an address is one the installation may
// treat as its own infrastructure — its own host, or its own network.
//
// Read by the sovereign profile check as well as by the egress classes, because
// they are the same question asked twice: "is this address ours?"
func customerControlled(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate()
}

// requireDialableEndpoint refuses a base_url the outbound dialer would refuse
// anyway, at the write instead of at the first call.
//
// Refused HERE for the reason ValidateTierBinding's openai_compatible rule is:
// accepted at the door it saves cleanly and the operator is told it worked,
// and the failure then arrives as an "unreachable" vendor with the reason in a
// log nobody is reading.
//
// A DNS name is not judged — what it resolves to is decided elsewhere and can
// change after boot, which is exactly why dialGuard checks the resolved address
// rather than trusting this.
func requireDialableEndpoint(label, provider, baseURL string) error {
	if baseURL == "" {
		return nil // no host of its own; the adapter's compiled default applies
	}
	if _, known := providerEgress[provider]; !known {
		// No lane to judge. SelectBrain refuses a provider this build has no
		// adapter for, so the binding reaches no socket at all — and answering
		// here would report an egress fault for a binding whose actual problem
		// is the provider name, which is the error the operator has to act on.
		// TestEveryProviderDeclaresAnEgressClass keeps this from ever skipping
		// a provider that CAN be served.
		return nil
	}
	parsed, err := parsedEndpoint(baseURL)
	if err != nil {
		return fmt.Errorf("ai: routing config: %s: %w", label, err)
	}
	if parsed.User != nil {
		return userinfoRefused(label, parsed)
	}
	// The address before the scheme, because a vendor binding pointed into this
	// deployment's own network is the more surprising of the two mistakes and
	// the one the operator has to understand first. Both are refused; only the
	// order of the message is decided here.
	if ip := parseHostAddress(parsed.Hostname()); ip != nil && !addressAllowed(egressFor(provider), ip) {
		return unreachableAddressRefused(label, provider, ip)
	}
	if egressFor(provider) == egressPublicOnly && !strings.EqualFold(parsed.Scheme, "https") {
		return cleartextRefused(label, provider, parsed.Scheme)
	}
	return nil
}

// userinfoRefused names the shape without echoing it: this error reaches a boot
// log, and the value being refused is by definition one carrying a credential.
func userinfoRefused(label string, parsed *url.URL) error {
	return fmt.Errorf(
		"ai: routing config: %s: base_url %q carries userinfo, and a binding never carries a credential — it would be sent to whatever host the value names. Give the host root alone; a model key belongs in the key vault",
		label, safeToName(parsed))
}

// cleartextRefused is the write-time half of refuseOffHostRedirect's downgrade
// rule. The two are one obligation: a vendor call carries this installation's
// model key, and a client that refuses to be redirected into clear while
// accepting a binding that starts there would be guarding one door of two.
//
// The vendor lane only. An operator's own gateway is reached over http all the
// time and the key travels no further than their network, so openai_compatible,
// ollama and vllm are untouched.
func cleartextRefused(label, provider, scheme string) error {
	return fmt.Errorf(
		"ai: routing config: %s: provider %q calls a vendor's public API with this installation's model key, so its base_url must be https — %q would send the key in clear. To reach a gateway on your own network over http, bind openai_compatible instead",
		label, provider, scheme)
}

// unreachableAddressRefused tells the operator which rule they met and what to
// do about it — two lanes, two different fixes.
func unreachableAddressRefused(label, provider string, ip net.IP) error {
	if egressFor(provider) == egressOperatorEndpoint {
		return fmt.Errorf(
			"ai: routing config: %s: base_url points at %s, which is not an address inference is served from — an endpoint is loopback, a private range (10.x, 172.16-31.x, 192.168.x, an IPv6 unique-local address), or a public host. Link-local, carrier-grade NAT and the documentation ranges are refused on every profile",
			label, ip)
	}
	return fmt.Errorf(
		"ai: routing config: %s: provider %q calls a vendor's public API and carries this installation's model key, so its base_url must name a public host — %s is not one. To reach a gateway on your own network, bind openai_compatible instead",
		label, provider, ip)
}
