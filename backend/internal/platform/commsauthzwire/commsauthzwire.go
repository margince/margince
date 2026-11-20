// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package commsauthzwire renders the engine's footer findings and cited law in
// the contract's shape. The send preview and the communication review both
// serve them, from two modules that may not import each other.
package commsauthzwire

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// FooterFindings renders footer findings with their provisions. Missing means
// are an empty list rather than null.
func FooterFindings(findings []commsauthz.FooterFinding) *[]crmcontracts.FooterFinding {
	out := make([]crmcontracts.FooterFinding, 0, len(findings))
	for _, f := range findings {
		means := f.MissingMeans
		if means == nil {
			means = []string{}
		}
		out = append(out, crmcontracts.FooterFinding{
			Kind: f.Kind, Language: f.Language, MissingMeans: means, Provision: Provision(f.Provision),
			Provisions: Provisions(f.Provisions),
		})
	}
	return &out
}

// Provision renders a citation, or nil when none is shown.
func Provision(p commsauthz.Provision) *crmcontracts.CitedProvision {
	if !p.Cited() {
		return nil
	}
	return &crmcontracts.CitedProvision{Instrument: p.Instrument, Provision: p.Provision, SourceUrl: p.SourceURL}
}

// Provisions renders every citation shown, or nil when none is.
func Provisions(cited []commsauthz.Provision) *[]crmcontracts.CitedProvision {
	out := make([]crmcontracts.CitedProvision, 0, len(cited))
	for _, p := range cited {
		if wire := Provision(p); wire != nil {
			out = append(out, *wire)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}
