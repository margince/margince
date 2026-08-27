// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The census this file exists for: a wildcard in a carriage declaration is a
// claim that the wire decodes every media type under a whole top-level type,
// and no vendor does. Where one remains it is because the endpoint is the
// operator's choice, and that is written down in wildcardWires rather than
// inferred — so an adapter added against a NEW vendor cannot keep `image/*` by
// saying nothing.
//
// Over knownProviders, which is the list SelectBrain switches on, so a provider
// that ships without a declaration fails here rather than being skipped.
func TestOnlyAnOperatorPointedWireDeclaresAWildcard(t *testing.T) {
	carriage := wireCarriage()
	for _, provider := range knownProviders {
		declared, described := carriage[provider]
		if !described {
			t.Errorf("%s ships and declares no carriage; wireCarriage must answer for every provider "+
				"SelectBrain builds, or the census below sees a smaller build than the one that runs", provider)
			continue
		}
		reason, allowed := wildcardWires[provider]
		switch {
		case declaresAWildcard(declared) && !allowed:
			t.Errorf("%s declares %v, and a wildcard claims every media type under its type — "+
				"name what the vendor documents it decodes, or add %s to wildcardWires with the reason "+
				"its endpoint is the operator's choice", provider, declared, provider)
		case !declaresAWildcard(declared) && allowed:
			t.Errorf("%s is in wildcardWires (%q) and declares no wildcard (%v) — the exemption is stale "+
				"and now hides the next one", provider, reason, declared)
		}
	}
	// The other direction: an exemption for a provider this build does not have
	// is an exemption nothing is checking.
	for provider := range wildcardWires {
		if !slices.Contains(knownProviders, provider) {
			t.Errorf("wildcardWires exempts %q, which is not a known provider", provider)
		}
	}
}

// Every media type a vendor list names must be one a vendor could send. The
// spelling is the whole contract with model.CarriesMIME — a subtype written
// with a trailing "*" would silently become a wildcard, and a bare type with no
// subtype matches nothing at all.
func TestEveryDeclaredMediaTypeIsSpelledLikeOne(t *testing.T) {
	for provider, declared := range wireCarriage() {
		for _, pattern := range declared {
			base, wildcard := strings.CutSuffix(pattern, "*")
			if !strings.Contains(base, "/") {
				t.Errorf("%s declares %q, which names no subtype and so matches nothing", provider, pattern)
			}
			if !wildcard && strings.Contains(pattern, "*") {
				t.Errorf("%s declares %q: a star anywhere but the end is a literal character to "+
					"CarriesMIME, so this matches only itself", provider, pattern)
			}
		}
	}
}

// DocumentMIMEs answers "could any binding have been handed this". A union, so
// a type only one vendor decodes still counts — the certification corpus is
// asking about the build, not about a particular binding.
func TestDocumentMIMEsCoversEveryAdaptersDeclaration(t *testing.T) {
	all := DocumentMIMEs()
	for provider, declared := range wireCarriage() {
		for _, pattern := range declared {
			if !model.CarriesMIME(all, pattern) {
				t.Errorf("%s carries %q and DocumentMIMEs (%v) does not admit it, so a corpus fixture "+
					"pinning it would be rejected as unreachable while %s could in fact be handed it",
					provider, pattern, all, provider)
			}
		}
	}
	// And nothing beyond them: a union that grew a type no adapter declares
	// would admit a fixture describing a call this build cannot make.
	for _, pattern := range all {
		carried := false
		for _, declared := range wireCarriage() {
			if slices.Contains(declared, pattern) {
				carried = true
				break
			}
		}
		if !carried {
			t.Errorf("DocumentMIMEs offers %q and no adapter declares it", pattern)
		}
	}
}

// The types the wildcard used to admit and no vendor decodes — the failure this
// whole change is about. Asserted at the boundary an operator actually meets:
// a binding built the way production builds one.
func TestAVendorBindingRefusesAnImageTypeItsVendorCannotDecode(t *testing.T) {
	undecodable := []string{"image/svg+xml", "image/bmp", "image/tiff"}
	for _, provider := range []string{providerAnthropic, providerOpenAI, providerGemini} {
		for _, mime := range undecodable {
			caps := capsFor(t, provider, nil)
			if model.CarriesMIME(caps, mime) {
				t.Errorf("%s advertises %q, which it cannot decode — the call goes out with that "+
					"media_type and comes back a vendor 400, instead of being refused here (%v)",
					provider, mime, caps)
			}
		}
	}
}
