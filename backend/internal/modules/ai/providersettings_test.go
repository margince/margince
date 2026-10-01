// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// A routing document written before hosts and upstream preferences moved onto
// the provider, in the two shapes installations store: an EU broker binding
// pinned on every lane, and a frontier binding with a capped tier and a
// decisions lane. The versions are literals taken from the code that wrote
// these documents, so a move that re-attributed every cached brief fails here.
func TestDigest_UnchangedBindingKeepsItsVersion(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"an EU broker binding pinned on every lane": {
			doc: `profile: eu_hosted
tiers:
  cheap_cloud: {provider: openai_compatible, model: mistralai/ministral-14b-2512, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu], require_parameters: true}}
  premium: {provider: openai_compatible, model: mistralai/mistral-medium-3-5, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu], require_parameters: true}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: "https://openrouter.ai/api", dimensions: 1024, routing: {only: [mistral/eu]}}
`,
			want: "cb4a84809fd2c0d0c466c60019c640d0bd9fc0ec5cdbfbe2a48f23600ac475e3",
		},
		"a frontier binding with a capped tier and a decisions lane": {
			doc: `profile: cloud_frontier
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
  cheap_cloud: {provider: openai_compatible, model: openai/gpt-oss-120b, base_url: "https://openrouter.ai/api", routing: {sort: throughput, quantizations: [bf16], reasoning_effort: low}}
  premium: {provider: openai_compatible, model: anthropic/claude-sonnet-4.5, base_url: "https://openrouter.ai/api", routing: {sort: throughput, quantizations: [bf16]}}
embeddings: {provider: openai_compatible, model: baai/bge-m3, base_url: "https://openrouter.ai/api"}
decisions: {provider: jev_compatible, model: typesafe/jev-1.13, base_url: "https://openrouter.ai/api/alpha/decisions"}
`,
			want: "498572ba4efcfbfd7e27eaeda5f259192d1cc99596e65cda4366ecbf86dfdfa6",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := versionOf(t, tc.doc); got != tc.want {
				t.Errorf("RoutingVersion = %s, want %s: the binding is unchanged, so every cached brief must stay attributed to it", got, tc.want)
			}
		})
	}
}
