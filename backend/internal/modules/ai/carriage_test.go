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

// wireCarriage is a SECOND copy of what the adapters declare, and a second copy
// is only safe while something fails when the two disagree.
//
// They did disagree. `openai_compatible` and `vllm` were listed as carrying
// `application/pdf` while no binding either word selects has ever carried one in
// any configuration — openAICompatMessages builds `image_url` parts and has no
// document part at all. Nothing computed a wrong answer from it, because
// DocumentMIMEs is a union and three native adapters contribute PDF anyway; the
// cost was a row a reader believes.
//
// The existing census could not see it: TestDocumentMIMEsCoversEveryAdaptersDeclaration
// checks the union covers each ROW, never that a row matches a BINDING. So the
// subject of this test is what SelectBrain actually constructs, and the map is
// held against it in both directions — a row that overstates its wire fails, and
// a provider the map forgets fails too.
func TestWireCarriageIsWhatSelectBrainActuallyBuilds(t *testing.T) {
	census := wireCarriage()
	for _, provider := range knownProviders {
		// An UNDECLARED binding: `input:` is what narrows AttachmentMIMEs, and
		// the wire's own answer is the question here.
		built := capsWireFor(t, provider)
		declared, described := census[provider]
		if !described {
			t.Errorf("%s ships and wireCarriage does not answer for it", provider)
			continue
		}
		if !slices.Equal(built, declared) {
			t.Errorf("%s builds a client declaring wire carriage %v and wireCarriage says %v — "+
				"the map is a second copy of the adapter's answer, and this is the drift that "+
				"makes the copy worse than nothing", provider, built, declared)
		}
	}
}

// An undeclared binding's wire carriage IS its ordinary carriage, and a declared
// one's is still the wire's. Both directions are asserted because the field only
// earns its place if narrowing leaves it alone — a WireAttachmentMIMEs that moved
// with `input:` would answer the same question AttachmentMIMEs already answers,
// and could never tell a closed lane from an absent one.
func TestNarrowingABindingLeavesItsWireDeclarationWhereItWas(t *testing.T) {
	for _, provider := range knownProviders {
		wire := capsWireFor(t, provider)
		narrowed := capsWireForInput(t, provider, []string{"text"})
		if !slices.Equal(wire, narrowed) {
			t.Errorf("%s reports wire carriage %v undeclared and %v once narrowed; "+
				"the wire does not change when an operator edits their config",
				provider, wire, narrowed)
		}
		// And the narrowed binding really did lose something, or the check above
		// passed by narrowing nothing.
		if len(wire) > 0 && len(capsFor(t, provider, []string{"text"})) > 0 {
			t.Errorf("%s declared `input: [text]` and still carries %v, so this case "+
				"never exercised a narrowing", provider, capsFor(t, provider, []string{"text"}))
		}
	}
}

// capsWireFor builds a binding with no `input:` and reports the wire it declares.
func capsWireFor(t *testing.T, provider string) []string {
	t.Helper()
	return capsWireForInput(t, provider, nil)
}

// capsWireForInput is capsWireFor with the declaration supplied. Through
// SelectBrain like capsFor, so what it reports is what a configuration ships.
func capsWireForInput(t *testing.T, provider string, input []string) []string {
	t.Helper()
	cfg := ProviderConfig{Provider: provider, Model: "m", Input: input}
	if provider == providerOpenAICompatible {
		cfg.BaseURL = "https://example.invalid" // the one provider that requires it
	}
	client, err := SelectBrain(cfg, allCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	return client.Caps().WireAttachmentMIMEs
}

// A wire declaration must never be NARROWER than what the binding says it
// carries: the whole point is that it is the ceiling, and a ceiling below the
// floor would report a carried type as withheld and stop a caller converting a
// document it was free to convert.
func TestAWireNeverDeclaresLessThanTheBindingOnIt(t *testing.T) {
	for _, provider := range knownProviders {
		for _, input := range [][]string{nil, {"text"}, {"text", "image"}} {
			wire, carried := capsWireForInput(t, provider, input), capsFor(t, provider, input)
			for _, pattern := range carried {
				if !model.CarriesMIME(wire, pattern) {
					t.Errorf("%s with input %v carries %q and declares a wire of %v that does not admit it",
						provider, input, pattern, wire)
				}
			}
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
