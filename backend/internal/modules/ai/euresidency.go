// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"strings"
)

// isEURegionHost reports whether a broker host slug names one EU-region
// endpoint.
//
// A base slug (`mistral`) matches every region the vendor serves from, and a
// variant slug (`mistral/zdr`) names a retention policy rather than a place, so
// neither pins anything to the EU.
func isEURegionHost(slug string) bool {
	_, region, found := strings.Cut(slug, "/")
	if !found {
		return false
	}
	return region == "eu" || strings.HasPrefix(region, "eu-") || strings.HasPrefix(region, "europe-")
}

// EURegionPinGap names why a broker binding may be served outside the EU, or
// answers "" when every host its `only:` admits is an EU-region endpoint.
//
// A broker fronts many hosts per model and, without `only:`, picks among them
// itself — a model with no EU endpoint at all is still served, from wherever it
// runs, and nothing fails. So on a broker the pin is the whole residency
// guarantee. A binding the broker does not front answers "": it names one host,
// and whether that host is in the EU is a fact about the host this rule cannot
// read.
func EURegionPinGap(binding ProviderConfig) string {
	if !UpstreamPreferencesApply(binding) {
		return ""
	}
	if binding.Routing == nil || len(binding.Routing.Only) == 0 {
		return "no `only:` — the broker may serve " + binding.Model + " from any region"
	}
	for _, slug := range binding.Routing.Only {
		if !isEURegionHost(slug) {
			return "`only:` admits " + slug + ", which is not an EU-region endpoint"
		}
	}
	return ""
}

// requireEURegionPin refuses a broker lane under eu_hosted that the broker may
// serve outside the EU.
//
// eu_hosted is the residency an operator chose, and the certification records
// are filed under it: a broker lane without an EU pin would read as EU inference
// on every screen while its text went wherever the broker sent it. The
// embeddings lane is held too, because it reads every document the chat tiers
// send.
func requireEURegionPin(profile Profile, lane string, binding ProviderConfig) error {
	if profile != ProfileEUHosted {
		return nil
	}
	if gap := EURegionPinGap(binding); gap != "" {
		return fmt.Errorf("ai: routing config: %s under profile eu_hosted: %s; "+
			"pin it with `routing: {only: [<vendor>/eu]}` to hosts the broker lists in the EU, "+
			"or declare profile cloud_frontier if this binding does not promise EU inference", lane, gap)
	}
	return nil
}

// ResidencyGap refuses a config whose profile is eu_hosted while a broker lane,
// the embeddings lane included, may be served outside the EU.
//
// It is held at every place a binding is WRITTEN — the routing file, a preset,
// a settings write — and deliberately not by finalize(), which a stored binding
// also passes through at boot. A binding stored before this rule existed would
// otherwise stop the installation's AI from loading on upgrade; the load path
// reports the same gap as a warning instead (compose.ResolveRouting), and the
// next write has to settle it.
func (cfg RoutingConfig) ResidencyGap() error {
	for tier, binding := range cfg.Tiers {
		if err := requireEURegionPin(cfg.Profile, fmt.Sprintf("tier %s", tier), binding); err != nil {
			return err
		}
	}
	return requireEURegionPin(cfg.Profile, "the embeddings lane", cfg.Embeddings.ProviderConfig)
}
