// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
)

// The routing version is a cache key. It travels into contactbrief.Fingerprint
// and its siblings, where it decides whether a stored brief may be reused, so
// what it must track is the BINDING — and what it must ignore is everything
// about how that binding was written down.

const baseRouting = `profile: eu_hosted
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite}
  premium: {provider: gemini, model: gemini-3.5-flash}
  frontier: {provider: gemini, model: gemini-3.1-pro-preview}
embeddings: {provider: gemini, model: gemini-embedding-001, dimensions: 1536}
`

func versionOf(t *testing.T, doc string) string {
	t.Helper()
	cfg, err := ParseRouting([]byte(doc))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	if cfg.RoutingVersion() == "" {
		t.Fatal("a parsed config has no routing version")
	}
	return cfg.RoutingVersion()
}

// Rewriting the file without changing what it routes must not invalidate a
// single cached brief. Each of these used to, because the digest was taken over
// the raw bytes: a comment cost an installation every brief it had.
func TestRewritingTheFileWithoutChangingTheBindingKeepsTheVersion(t *testing.T) {
	want := versionOf(t, baseRouting)

	for name, doc := range map[string]string{
		"a comment is added": "# why we run on gemini, for the next reader\n" + baseRouting,
		"the tiers are reordered": `profile: eu_hosted
tiers:
  frontier: {provider: gemini, model: gemini-3.1-pro-preview}
  premium: {provider: gemini, model: gemini-3.5-flash}
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite}
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
embeddings: {provider: gemini, model: gemini-embedding-001, dimensions: 1536}
`,
		"the same binding is written in block style": `profile: eu_hosted
tiers:
  local_small:
    provider: gemini
    model: gemini-3.1-flash-lite
  cheap_cloud:
    provider: gemini
    model: gemini-3.1-flash-lite
  premium:
    provider: gemini
    model: gemini-3.5-flash
  frontier:
    provider: gemini
    model: gemini-3.1-pro-preview
embeddings:
  provider: gemini
  model: gemini-embedding-001
  dimensions: 1536
`,
		// The default is applied before the digest is taken, so saying it out
		// loud and leaving it out are the same binding — which is what an
		// operator would assume, and what the byte digest punished.
		"the default width is written out instead of omitted": strings.Replace(
			baseRouting, ", dimensions: 1536", "", 1,
		),
		"a trailing blank line": baseRouting + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := versionOf(t, doc); got != want {
				t.Errorf("the routing version changed but the binding did not:\n got  %s\n want %s", got, want)
			}
		})
	}
}

// The other half, and the one that matters more: a digest that ignored too much
// would leave a brief attributed to a model that no longer wrote it. Content a
// model produced must stop being reused the moment the lane is re-pointed.
func TestChangingTheBindingChangesTheVersion(t *testing.T) {
	base := versionOf(t, baseRouting)

	for name, doc := range map[string]string{
		"a tier is re-pointed at another model": strings.Replace(
			baseRouting, "premium: {provider: gemini, model: gemini-3.5-flash}",
			"premium: {provider: gemini, model: gemini-3.1-pro-preview}", 1,
		),
		"the embeddings width changes": strings.Replace(
			baseRouting, "dimensions: 1536", "dimensions: 768", 1,
		),
		"the location ladder changes": strings.Replace(
			baseRouting, "profile: eu_hosted", "profile: cloud_frontier", 1,
		),
		"a tier gains a base-url override": strings.Replace(
			baseRouting, "premium: {provider: gemini, model: gemini-3.5-flash}",
			"premium: {provider: gemini, model: gemini-3.5-flash, base_url: https://eu-gateway.example}", 1,
		),
		"a tier narrows what it accepts": strings.Replace(
			baseRouting, "premium: {provider: gemini, model: gemini-3.5-flash}",
			"premium: {provider: gemini, model: gemini-3.5-flash, input: [text]}", 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if got := versionOf(t, doc); got == base {
				t.Error("the binding changed but the routing version did not — cached content stays attributed to a model that no longer produces it")
			}
		})
	}
}

// The digest is compared across processes and across restarts, so it has to be
// a function of the binding alone. Go sorts map keys and orders struct fields
// by declaration when marshaling, which is what makes this hold for a config
// whose tiers are a map.
func TestTheVersionIsStableAcrossRepeatedParses(t *testing.T) {
	first := versionOf(t, baseRouting)
	for range 32 {
		if got := versionOf(t, baseRouting); got != first {
			t.Fatalf("the routing version is not deterministic: got %s, then %s", first, got)
		}
	}
}

// Every installation's cached briefs, dossiers and growth-fits are keyed on
// these digests, so a change to how a binding marshals regenerates them all
// through paid models. The hex values are pinned, not recomputed: a renamed
// constant or a new field without omitempty fails here instead of in a bill.
func TestAnExistingBindingKeepsItsPinnedVersion(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"a single-vendor cloud binding": {
			doc:  baseRouting,
			want: "9f63f2eed1f2f0fa302b9b5c3ff0a5ffe2022324f5bf3d041dd8cd53b9c180f2",
		},
		"a brokered binding with a defaulted upstream and a narrowed input": {
			doc: `profile: eu_hosted
tiers:
  premium: {provider: openai_compatible, base_url: https://openrouter.ai/api, model: mistralai/mistral-small-2603}
  frontier: {provider: anthropic, model: claude-sonnet-4-5, input: [text, image]}
embeddings: {provider: gemini, model: gemini-embedding-001}
`,
			want: "e9c868909349fac6f7aadea107a94281362a37b95f1f7cc5a320193142972377",
		},
		"a Vertex binding, which alone marshals a location": {
			doc: `profile: eu_hosted
tiers:
  premium: {provider: gemini_vertex, location: eu, model: gemini-3.5-flash}
embeddings: {provider: gemini_vertex, location: europe-west4, model: gemini-embedding-001}
`,
			want: "70df0c5068f432441c98f6af9b4ae5c6dfb432652b92f04f4d3e5d1c0023602d",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := versionOf(t, tc.doc); got != tc.want {
				t.Errorf("an unchanged binding hashes differently, which regenerates every cached AI output:\n got  %s\n want %s", got, tc.want)
			}
		})
	}
}
