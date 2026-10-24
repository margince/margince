// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func buildCompanySurvivorshipPatch(target, source crmcontracts.Company) *storekit.Patch {
	p := storekit.NewPatch()
	fillString(p, fieldLegalName, target.LegalName, source.LegalName)
	fillString(p, "description", target.Description, source.Description)
	fillIndustry(p, target, source)
	fillString(p, "linkedin_url", target.LinkedinUrl, source.LinkedinUrl)
	fillMergeAddress(p, target.Address, source.Address)
	return p
}

// An address moves as a block; filling individual gaps can invent a location.
func fillMergeAddress(p *storekit.Patch, target, source *crmcontracts.Address) {
	if target != nil || source == nil {
		return
	}
	p.Set("address_line1", nil, source.Line1)
	p.Set("address_line2", nil, source.Line2)
	p.Set("address_city", nil, source.City)
	p.Set("address_region", nil, source.Region)
	p.Set("address_postal_code", nil, source.PostalCode)
	p.Set("address_country", nil, source.Country)
}

// fillIndustry gives the survivor the retired company's industry through the
// vocabulary owner, so a legacy text becomes a code, or only a source note.
func fillIndustry(p *storekit.Patch, target, source crmcontracts.Company) {
	if target.Industry != nil || source.Industry == nil {
		return
	}
	raw := string(*source.Industry)
	write := resolveIndustryWrite(&raw)
	if write.code != nil {
		p.Set("industry", nil, *write.code)
		return
	}
	base := target.Description
	if base == nil {
		base = source.Description
	}
	if next, changed := withIndustrySource(base, raw); changed {
		p.Set("description", target.Description, next)
	}
}
