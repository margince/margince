// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package providerwire renders provider values the way the contract spells them.
// Two readers put a run on the wire: the run endpoint and the contact page.
package providerwire

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// Snapshot renders the frozen configuration in the shape the live configuration
// uses. A caller can then compare what a run was allowed with what the
// connection allows now.
func Snapshot(s provider.Snapshot) crmcontracts.ProviderConfiguration {
	cats := crmcontracts.ProviderCategorySelection{}
	for _, c := range s.Categories {
		cats[string(c)] = true
	}
	return crmcontracts.ProviderConfiguration{
		Mode:                      crmcontracts.ProviderConnectionMode(s.Mode),
		Preset:                    s.Preset,
		AutomaticIndividualCreate: s.AutomaticCreate,
		AutomaticImport:           s.AutomaticImport,
		Categories:                cats,
		RefreshAfterDays:          s.RefreshAfterDays,
		DailyRunLimit:             s.DailyRunLimit,
	}
}
