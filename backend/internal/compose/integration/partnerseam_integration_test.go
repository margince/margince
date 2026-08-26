// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The partner extension through the datasource seam — the surface an MCP
// agent reads partners by.
//
// A partner is the 1:1 extension of an organization, so the seam addresses it
// by the ORGANIZATION's id. That is the detail worth holding: a reader who
// cannot open the company must not learn its commercial terms through the
// other name, and the seam must not offer a second way in that skips the gate
// the HTTP handler applies.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func partnerSeamProvider(e *Env) *people.Provider { return people.NewProvider(e.DB()) }

// partnerReader is a seat holding the partner and organization grants the
// read needs. AdminPerms carries no `partner` object on purpose (see
// SeedPartnerOrg), so a suite that wants the ADMIT case must say so.
func partnerReader() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects: map[string]principal.ObjectGrant{
			"partner":      {Read: true},
			"organization": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
}

// The admit case first: without it, a refusal test proves only that the
// authority refuses everyone.
func TestPartnerReadsThroughTheSeamByItsOrganizationID(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	tier := "tier2_20"
	org := e.SeedPartnerOrg(t, "Seam Partner GmbH", &tier, nil)
	ctx := e.As(e.AdminUser, nil, partnerReader())

	rec, err := p.Read(ctx, datasource.EntityRef{Type: datasource.EntityPartner, ID: org})
	if err != nil {
		t.Fatalf("reading a partner through the seam: %v", err)
	}
	if rec.Ref.Type != datasource.EntityPartner {
		t.Fatalf("record type = %s, want partner", rec.Ref.Type)
	}
	if rec.Ref.ID != org {
		t.Fatalf("record id = %s, want the organization id %s", rec.Ref.ID, org)
	}
}

// The seam is not a weaker copy of the HTTP path: a seat with no partner
// grant is refused here too.
func TestPartnerSeamRefusesASeatWithoutThePartnerGrant(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	tier := "tier1_15"
	org := e.SeedPartnerOrg(t, "Ungranted GmbH", &tier, nil)

	// AdminPerms deliberately holds no `partner` object.
	_, err := p.Read(e.Admin(), datasource.EntityRef{Type: datasource.EntityPartner, ID: org})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read without the partner grant = %v, want ErrPermissionDenied", err)
	}
}

// An organization that never joined the programme has no partner row, and the
// seam says not-found rather than answering an empty record.
func TestPartnerSeamIsNotFoundForAPlainCompany(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	org := e.SeedOrg(t, "Just A Customer GmbH", nil)
	ctx := e.As(e.AdminUser, nil, partnerReader())

	_, err := p.Read(ctx, datasource.EntityRef{Type: datasource.EntityPartner, ID: org})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a non-partner company = %v, want ErrNotFound", err)
	}
}

// ListPartners has no text index. A query term is refused rather than dropped,
// because a dropped term answers an UNFILTERED page that reads as "these all
// matched".
func TestPartnerSeamRefusesATextQuery(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	tier := "tier3_25"
	e.SeedPartnerOrg(t, "Findable GmbH", &tier, nil)
	ctx := e.As(e.AdminUser, nil, partnerReader())

	term := "Findable"
	_, _, _, err := p.SearchEntity(ctx, datasource.EntityPartner, &term, 10, nil, nil)
	if err == nil {
		t.Fatal("a text query against partner was accepted; it must be refused, not silently dropped")
	}
}

// The role and certification dials are the whole vocabulary, and they narrow.
func TestPartnerSeamNarrowsByRole(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	tier := "tier2_20"
	e.SeedPartnerOrg(t, "Consulting GmbH", &tier, nil)
	ctx := e.As(e.AdminUser, nil, partnerReader())

	records, _, _, err := p.SearchEntity(ctx, datasource.EntityPartner, nil, 10, nil,
		map[string]string{"partner_role": "consulting"})
	if err != nil {
		t.Fatalf("listing partners by role: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("no partners came back for partner_role=consulting, but one was seeded with it")
	}
	for _, rec := range records {
		if rec.Ref.Type != datasource.EntityPartner {
			t.Fatalf("listed record type = %s, want partner", rec.Ref.Type)
		}
	}

	// A role nobody holds is an empty page, not every partner.
	none, _, _, err := p.SearchEntity(ctx, datasource.EntityPartner, nil, 10, nil,
		map[string]string{"partner_role": "hosting"})
	if err != nil {
		t.Fatalf("listing partners by an unheld role: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("partner_role=hosting returned %d rows, want none — the filter is not narrowing", len(none))
	}
}

// A filter this type cannot answer is refused, so a caller never reads an
// unfiltered page as a filtered one.
func TestPartnerSeamRefusesAnUnknownFilter(t *testing.T) {
	e := Setup(t)
	p := partnerSeamProvider(e)
	ctx := e.As(e.AdminUser, nil, partnerReader())

	_, _, _, err := p.SearchEntity(ctx, datasource.EntityPartner, nil, 10, nil,
		map[string]string{"owner_id": ids.NewV7().String()})
	if err == nil {
		t.Fatal("owner_id was accepted for partner; a filter the type cannot answer must be refused")
	}
}
