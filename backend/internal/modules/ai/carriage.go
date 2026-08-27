// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"strings"
)

// What each wire carries, in model.CarriesMIME's spelling.
//
// The rule these declarations follow: an adapter bound to ONE vendor names the
// media types that vendor documents it decodes, and an adapter pointed at an
// endpoint the OPERATOR chose keeps the wildcard, because the decoder is the
// operator's model rather than ours.
//
// The wildcard was the wrong answer for the first group, and wrong in the
// direction capability negotiation exists to prevent. `image/*` admits
// image/svg+xml, image/bmp and image/tiff; no vendor here decodes any of them.
// An attachment labelled that way passed Caps(), passed attachmentUnsupported,
// went on the wire with that media_type and came back a vendor 400 — the shape
// the negotiation is supposed to refuse before the call, with a sentinel a
// caller can route on.
//
// Narrowing these was only possible once model.IntersectMIMEs read patterns
// rather than spellings: a ladder mixing a narrowed rung with a wildcard rung
// used to intersect to nothing and lose its document lane silently.

// The media types the declarations below are written from. Constants because a
// typo in a media type is not a compile error — it is a lane that quietly
// refuses everything, or one that quietly refuses nothing.
//
// Naming a spelling is not sharing a list: each vendor's declaration is still
// composed on its own, so narrowing one cannot narrow another.
const (
	mimeAnyImage = "image/*"
	mimeJPEG     = "image/jpeg"
	mimePNG      = "image/png"
	mimeGIF      = "image/gif"
	mimeWebP     = "image/webp"
	mimeHEIC     = "image/heic"
	mimeHEIF     = "image/heif"
	mimePDF      = "application/pdf"
)

// carriesNothing is the carriage declaration of an adapter whose wire has no
// attachment parts at all. Named rather than written as a bare nil at each
// site, so "this wire is text-only" reads as a decision instead of an omission.
var carriesNothing []string

// anthropicCarries is the Messages API's image block plus its document block:
// the four image types the image source accepts, and PDF.
var anthropicCarries = []string{mimeJPEG, mimePNG, mimeGIF, mimeWebP, mimePDF}

// openAICarries is what the Responses API's input_image accepts, plus the PDF
// its input_file takes. The same four images as Anthropic, which is a
// coincidence of two vendors converging rather than one list serving both —
// they are written twice on purpose, so narrowing one cannot narrow the other
// by accident.
var openAICarries = []string{mimeJPEG, mimePNG, mimeGIF, mimeWebP, mimePDF}

// geminiCarries is the inline-data set the Gemini API documents. It is NOT the
// same four: Gemini takes HEIC and HEIF, which neither other vendor does, and
// does not document GIF, which both others do. That divergence is the whole
// argument against a single shared constant.
var geminiCarries = []string{mimeJPEG, mimePNG, mimeWebP, mimeHEIC, mimeHEIF, mimePDF}

// carriesImagesAndPDF is the widest document carriage any adapter in this build
// has, and belongs to the adapters whose endpoint is the operator's choice: the
// injected fake, which stands in for whichever binding named it. What the
// endpoint behind it decodes is not knowable here, so this stays a claim about
// the WIRE's shape — it has image parts and document parts — rather than about
// a decoder.
var carriesImagesAndPDF = []string{mimeAnyImage, mimePDF}

// carriesImages is the declaration of an adapter whose wire takes images and
// nothing else: the Ollama chat API, which spells an image as an entry in a
// per-message `images` array and has no document part at all. Not a narrowing
// of carriesImagesAndPDF but a different wire — there is no document lane here
// to give a binding, which is why `input: [text, image]` on an ollama binding
// buys nothing and takes nothing away.
//
// Still a wildcard after the narrowing pass, and deliberately: Ollama serves
// whichever vision model the operator pulled, and what that model's projector
// was built for is a property of the pull, not of this adapter.
var carriesImages = []string{mimeAnyImage}

// wildcardWires names every adapter that keeps a wildcard in its declaration,
// with the reason it is not a vendor list. Written down so the census below
// reads a decision rather than an omission, and so adding a vendor adapter that
// forgets to narrow has to argue with this list.
var wildcardWires = map[string]string{
	ProviderFake:             "stands in for whichever binding named it, so it claims the wire's shape rather than a decoder",
	providerOllama:           "serves whichever vision model the operator pulled",
	providerVLLM:             "serves whichever model the operator loaded",
	providerOpenAICompatible: "serves whichever vendor the operator pointed base_url at",
}

// DocumentMIMEs is every media type some adapter in this build carries as an
// input part. It answers "could any binding have been handed this", which is a
// different question from "will THIS binding take it" (that is Caps()) — the
// certification corpus asks the first, because a fixture pinning a media type
// no adapter accepts describes a call this build cannot make.
//
// A union over the declarations rather than a copy of one of them: they stopped
// agreeing when each vendor started naming its own decoder, and a copy of the
// widest would have quietly become a copy of one vendor's answer.
//
// Held by: TestDocumentMIMEsCoversEveryAdaptersDeclaration (backend/internal/modules/ai/carriage_test.go)
func DocumentMIMEs() []string {
	var all []string
	for _, declaration := range wireCarriage() {
		for _, pattern := range declaration {
			if !slices.Contains(all, pattern) {
				all = append(all, pattern)
			}
		}
	}
	return all
}

// wireCarriage is every shipping adapter's declaration, keyed by the provider
// word that selects it. The census and DocumentMIMEs both read it, so neither
// can go looking at a set of adapters the other does not.
//
// Held by: TestOnlyAnOperatorPointedWireDeclaresAWildcard (backend/internal/modules/ai/carriage_test.go)
func wireCarriage() map[string][]string {
	return map[string][]string{
		ProviderFake:             carriesImagesAndPDF,
		providerAnthropic:        anthropicCarries,
		providerOllama:           carriesImages,
		providerVLLM:             carriesImagesAndPDF,
		providerOpenAICompatible: carriesImagesAndPDF,
		providerOpenAI:           openAICarries,
		providerGemini:           geminiCarries,
	}
}

// declaresAWildcard reports whether a carriage declaration leaves a media type
// unnamed — the property the census is about, asked once so the test and any
// future caller cannot spell it two ways.
func declaresAWildcard(declared []string) bool {
	for _, pattern := range declared {
		if _, wildcard := strings.CutSuffix(pattern, "*"); wildcard {
			return true
		}
	}
	return false
}

// isImage selects which KIND of wire part an accepted attachment becomes, once
// carriage has already been decided. It is not a carriage check — that is
// model.CarriesMIME over the adapter's declaration — and using it as one is how
// the two answers drift apart.
func isImage(mime string) bool { return strings.HasPrefix(mime, "image/") }
