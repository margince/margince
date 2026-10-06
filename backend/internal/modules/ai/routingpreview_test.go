// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"
)

// A refused connection key is restored to the stored one, like a refused tier
// routing: the preview still shows what the rest of the draft would send.
func TestAPreviewRestoresARefusedConnectionUpstream(t *testing.T) {
	stored, err := ParseRouting([]byte("profile: cloud_frontier\n" +
		"tiers:\n  premium: {provider: openai_compatible, model: m}\n" +
		"embeddings: {provider: openai_compatible, model: e}\n" +
		"providers:\n  openai_compatible:\n    base_url: https://openrouter.ai/api\n    upstream: {provider: {zdr: true}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	next := stored.canonical()
	bad := next.Providers["openai_compatible"]
	bad.Upstream = &OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: SortPrice}}}
	next.Providers = map[string]ProviderSettings{"openai_compatible": bad}
	// One more change elsewhere in the draft, which the preview must keep.
	premium := next.Tiers[TierPremium]
	premium.Model = "m-next"
	next.Tiers = map[Tier]ProviderConfig{TierPremium: premium}

	_, served, faults, err := previewDraft(stored, next, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !faults.under("providers.openai_compatible.upstream") {
		t.Fatalf("faults %+v, want the connection's sort refused", faults)
	}
	if got := served.Tiers[TierPremium].Model; got != "m-next" {
		t.Fatalf("premium serves %q, want the draft's m-next: the preview fell back to the stored document", got)
	}
	if got := served.Providers["openai_compatible"].Upstream; got == nil || got.Provider.ZDR == nil || !*got.Provider.ZDR {
		t.Errorf("the connection served %+v, want the stored zdr back", got)
	}
}
