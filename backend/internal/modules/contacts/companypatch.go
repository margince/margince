// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The company edit, folded onto the row it changes. Its own file because
// the checks it carries — a size band's vocabulary, a parent that must be
// visible before the edge lands — belong to the patch rather than to the store
// around it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// buildCompanyPatch folds the caller's sparse company edit into a patch.
// Naming a new parent is a read of that parent (the create-path rule), so
// it is visibility-probed before the edge lands.
func buildCompanyPatch(ctx context.Context, tx pgx.Tx, current crmcontracts.Company, in UpdateCompanyInput) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	if err := storekit.ApplyClears(p, in.Clear, clearableCompanyColumns(current)); err != nil {
		return nil, err
	}
	setCompanyPlainFields(p, current, in)
	if err := setCompanyCheckedFields(ctx, tx, p, current, in); err != nil {
		return nil, err
	}
	if in.Address != nil {
		cur := addressColumns(current.Address)
		p.Set("address_line1", cur.Line1, in.Address.Line1)
		p.Set("address_line2", cur.Line2, in.Address.Line2)
		p.Set("address_city", cur.City, in.Address.City)
		p.Set("address_region", cur.Region, in.Address.Region)
		p.Set("address_postal_code", cur.PostalCode, in.Address.PostalCode)
		p.Set("address_country", cur.Country, in.Address.Country)
	}
	return p, nil
}

// setCompanyPlainFields stages the fields whose only rule is "the caller
// supplied it". Kept apart from the checked ones so a reader can see at a
// glance which fields carry a rule and which do not.
func setCompanyPlainFields(p *storekit.Patch, current crmcontracts.Company, in UpdateCompanyInput) {
	if in.DisplayName != nil {
		p.Set("display_name", current.DisplayName, *in.DisplayName)
	}
	if in.LegalName != nil {
		p.Set("legal_name", current.LegalName, *in.LegalName)
	}
	if in.Description != nil {
		p.Set("description", current.Description, *in.Description)
	}
	if in.Industry != nil {
		p.Set("industry", current.Industry, *in.Industry)
	}
	if in.OwnerID != nil {
		p.Set(ownerIDColumn, current.OwnerId, *in.OwnerID)
	}
}

// setCompanyCheckedFields stages the fields that carry a rule: a closed
// vocabulary, a normalisation, or a link target that must be visible before the
// edge lands.
func setCompanyCheckedFields(ctx context.Context, tx pgx.Tx, p *storekit.Patch, current crmcontracts.Company, in UpdateCompanyInput) error {
	if in.SizeBand != nil {
		if err := checkSizeBand(*in.SizeBand); err != nil {
			return err
		}
		p.Set("size_band", current.SizeBand, *in.SizeBand)
	}
	if in.Lifecycle != nil {
		if err := checkLifecycle(*in.Lifecycle); err != nil {
			return err
		}
		p.Set("lifecycle", lifecycleValue(current.Lifecycle), *in.Lifecycle)
	}
	if in.ParentCompanyID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "company", in.ParentCompanyID.UUID); err != nil {
			return err
		}
		p.Set("parent_company_id", current.ParentCompanyId, *in.ParentCompanyID)
	}
	if in.LinkedInURL != nil {
		normalized, err := companyLinkedInPatchValue(*in.LinkedInURL)
		if err != nil {
			return err
		}
		p.Set("linkedin_url", current.LinkedinUrl, normalized)
	}
	return nil
}
