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

// The row that was false, held by the thing that made it false.
//
// wireCarriage listed openai_compatible and vllm as carrying `application/pdf`.
// The refutation is not another declaration agreeing with this one — both sides
// of that comparison are the same variable, and a mirror holds nothing. It is
// the PART BUILDER: openAICompatMessages emits `text` and `image_url` parts and
// has no document part at all, so a PDF handed to this wire cannot travel as a
// document however the binding is configured.
//
// Derived from the builder rather than restated, per AGENTS "a gate that
// hard-codes part of its subject has become a second copy of it".
func TestTheOpenAICompatibleWireHasNoDocumentPart(t *testing.T) {
	msgs := openAICompatMessages(
		"read documents", []model.Message{{Role: roleUser, Content: "what does it say"}},
		[]model.Attachment{{MIME: mimePDF, Bytes: []byte("%PDF-1.4"), Name: "invoice.pdf"}},
	)

	kinds := map[string]bool{}
	for _, message := range msgs {
		for _, part := range message.Content.Parts {
			kinds[part.Type] = true
		}
	}
	for kind := range kinds {
		if kind != "text" && kind != "image_url" {
			t.Fatalf("this wire grew a %q part; wireCarriage and this test both "+
				"assume text and image_url are the only two", kind)
		}
	}
	// A PDF reached the builder and came back as an IMAGE, which is the shape
	// the false row would have licensed a caller to produce.
	if !kinds["image_url"] {
		t.Fatal("the fixture attachment produced no part at all, so this proves nothing")
	}
	// Therefore the declaration may name no document type.
	for _, pattern := range wireCarriage()[providerOpenAICompatible] {
		if !strings.HasPrefix(pattern, "image/") {
			t.Errorf("openai_compatible declares %q and its wire can only build an image part, "+
				"so a caller told this is carried would have the document sent as a picture of nothing",
				pattern)
		}
	}
	// vllm is the SAME adapter under another provider word, so it inherits the
	// conclusion — asserted rather than assumed, because the two rows are written
	// separately and only one of them is reached by the loop above.
	if !slices.Equal(wireCarriage()[providerVLLM], wireCarriage()[providerOpenAICompatible]) {
		t.Errorf("vllm declares %v and openai_compatible %v; they are one adapter and one wire",
			wireCarriage()[providerVLLM], wireCarriage()[providerOpenAICompatible])
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
