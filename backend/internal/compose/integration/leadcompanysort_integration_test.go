// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A linked lead orders by its company's current name, the one its cell shows,
// and not by the name typed when the lead was filed.
func TestTheLeadsListSortsALinkedLeadByItsCompanysCurrentName(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	ctx := e.Admin()

	bravo := seedLeadTypingCompany(t, e, "First lead", "Bravo Co")
	linked := seedLeadTypingCompany(t, e, "Second lead", "Acme Ltd")
	yankee := seedLeadTypingCompany(t, e, "Third lead", "Yankee Co")
	company := e.SeedCompany(t, "Acme Ltd", &e.Rep1)
	if _, err := e.Contacts.LinkLeadCompany(ctx, ids.From[ids.LeadKind](linked), ids.From[ids.CompanyKind](company)); err != nil {
		t.Fatalf("linking the lead: %v", err)
	}
	if _, err := e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](company),
		contacts.UpdateCompanyInput{DisplayName: new("Zulu Ltd")}); err != nil {
		t.Fatalf("renaming the company: %v", err)
	}

	assertIDOrder(t, leadIDsIn(ctx, t, e, "company_name"), []ids.UUID{bravo, yankee, linked},
		"company ascending: Bravo, Yankee, then the lead whose company is now Zulu")
	assertIDOrder(t, leadIDsIn(ctx, t, e, "-company_name"), []ids.UUID{linked, yankee, bravo},
		"company descending: Zulu first")
}

// A linked company the reader may not see orders its lead by the typed name,
// as if the lead were not linked. Its position must not reveal the hidden name.
func TestALeadLinkedToAnUnreadableCompanySortsByTheTypedName(t *testing.T) {
	t.Parallel()
	e := Setup(t)

	mercator := seedLeadTypingCompany(t, e, "First lead", "Mercator")
	secret := seedLeadTypingCompany(t, e, "Second lead", "Zeta typed")
	hidden := e.SeedCompany(t, "Zeta typed", &e.Rep3)
	if _, err := e.Contacts.LinkLeadCompany(e.Admin(), ids.From[ids.LeadKind](secret), ids.From[ids.CompanyKind](hidden)); err != nil {
		t.Fatalf("linking the lead: %v", err)
	}
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](hidden),
		contacts.UpdateCompanyInput{DisplayName: new("Alma Werke")}); err != nil {
		t.Fatalf("renaming the company: %v", err)
	}
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)

	perms := AccountRepPerms
	perms.Objects = maps.Clone(AccountRepPerms.Objects)
	perms.Objects["lead"] = principal.ObjectGrant{Read: true}
	// The company's owner sees it, so its name orders the lead first.
	owner := perms
	owner.RowScope = principal.RowScopeAll
	assertIDOrder(t, leadIDsIn(e.As(e.Rep3, nil, owner), t, e, "company_name"), []ids.UUID{secret, mercator},
		"company ascending for a reader who sees Alma Werke")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	assertIDOrder(t, leadIDsIn(rep, t, e, "company_name"), []ids.UUID{mercator, secret},
		"company ascending for a reader who may not see the company: Mercator before Zeta typed")
	assertIDOrder(t, leadIDsIn(rep, t, e, "-company_name"), []ids.UUID{secret, mercator},
		"company descending: Zeta typed first")

	// Without the company grant at all, the typed name orders the page too.
	perms.Objects = map[string]principal.ObjectGrant{"lead": {Read: true}}
	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	assertIDOrder(t, leadIDsIn(blind, t, e, "company_name"), []ids.UUID{mercator, secret},
		"company ascending for a reader without company read")
}

// seedLeadTypingCompany creates a lead owned by Rep1 that names a company by
// typed text alone.
func seedLeadTypingCompany(t *testing.T, e *Env, name, company string) ids.UUID {
	t.Helper()
	l, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, CompanyName: &company, Source: "manual", OwnerID: userIDPtr(&e.Rep1),
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	return ids.UUID(l.Id)
}
