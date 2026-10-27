// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead's company facts over a real Postgres. The link offer tries the domain first; a read
// shows the facts, the list narrows by them, and a promotion carries the industry.

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// factCompany creates a company with the facts a lead can read off it.
func (e *privacyEnv) factCompany(ctx context.Context, t *testing.T, in CreateCompanyInput) ids.CompanyID {
	t.Helper()
	in.Source = "manual"
	company, err := e.store.CreateCompany(ctx, in)
	if err != nil {
		t.Fatalf("create company %s: %v", in.DisplayName, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

// factLead creates an owned lead from in.
func (e *privacyEnv) factLead(ctx context.Context, t *testing.T, owner ids.UUID, in CreateLeadInput) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	in.OwnerID, in.Source = &ownerID, "manual"
	if in.FullName == nil {
		in.FullName = new("Lena Lead")
	}
	lead, _, err := e.store.CreateLead(ctx, in)
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

// listedUnder is the leads both reads of the list answer for filters: the
// default work queue and a sorted page. The two must agree.
func (e *privacyEnv) listedUnder(ctx context.Context, t *testing.T, filters LeadCompanyFactFilters) ([]ids.UUID, error) {
	t.Helper()
	var answers [][]ids.UUID
	for _, sort := range []*string{nil, new("created_at")} {
		leads, _, err := e.store.ListLeads(ctx, ListLeadsInput{CompanyFacts: filters, Sort: sort})
		if err != nil {
			return nil, err
		}
		listed := make([]ids.UUID, 0, len(leads))
		for _, l := range leads {
			listed = append(listed, ids.UUID(l.Id))
		}
		slices.SortFunc(listed, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
		answers = append(answers, listed)
	}
	if !slices.Equal(answers[0], answers[1]) {
		t.Errorf("the work queue listed %v and the sorted page %v for %+v", answers[0], answers[1], filters)
	}
	return answers[0], nil
}

func TestTheLinkOfferMatchesTheClaimedDomainBeforeTheName(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	byDomain := e.factCompany(ctx, t, CreateCompanyInput{
		DisplayName: "Nordwind Holding", Domains: []CompanyDomainInput{{Domain: "nordwind.test", IsPrimary: true}},
	})
	byName := e.factCompany(ctx, t, CreateCompanyInput{DisplayName: "Nordwind Logistik"})
	lead := e.factLead(ctx, t, e.owner, CreateLeadInput{
		Email: new("ana@nordwind.test"), CompanyName: new("Nordwind Logistik"), CandidateCompanyKey: new("nordwind.test"),
	})

	offered, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if offered.CompanyMatch == nil || ids.UUID(offered.CompanyMatch.Id) != byDomain.UUID ||
		offered.CompanyMatch.MatchedBy != crmcontracts.LeadCompanyMatchMatchedByDomain {
		t.Fatalf("company_match = %+v, want the company claiming nordwind.test, matched by domain", offered.CompanyMatch)
	}
	_, err = e.store.LinkLeadCompany(ctx, lead, byName)
	if field := faultedField(err); field != leadCompanyIDColumn {
		t.Fatalf("confirming the same-name company the offer passed over = %v, want a refusal naming company_id", err)
	}
	linked, err := e.store.LinkLeadCompany(ctx, lead, byDomain)
	if err != nil {
		t.Fatalf("confirming the domain match: %v", err)
	}
	if linked.CompanyId == nil || ids.UUID(*linked.CompanyId) != byDomain.UUID {
		t.Errorf("company_id = %v, want %s", linked.CompanyId, byDomain)
	}
	again, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if again.CompanyMatch != nil {
		t.Errorf("a linked lead still offers %+v", again.CompanyMatch)
	}
	renamed, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{CompanyName: new("Nordwind Seefracht")})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.CompanyId == nil {
		t.Error("a rename dropped the link although the company still claims the lead's domain")
	}
}

// A link kept by a domain is re-checked when the lead's key moves or is cleared, as when its name moves.
func TestALinkByDomainFollowsTheLeadsKey(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.factCompany(ctx, t, CreateCompanyInput{
		DisplayName: "Gull Shipping", Domains: []CompanyDomainInput{{Domain: "gull.test", IsPrimary: true}},
	})
	for _, tc := range []struct {
		name  string
		patch UpdateLeadInput
	}{
		{"the key moves to another domain", UpdateLeadInput{CandidateCompanyKey: new("tern.test")}},
		{"the key is cleared", UpdateLeadInput{Clear: []string{"candidate_company_key"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lead := e.factLead(ctx, t, e.owner, CreateLeadInput{
				Email: new(ids.NewV7().String() + "@gull.test"), CompanyName: new("Gull Logistics"), CandidateCompanyKey: new("gull.test"),
			})
			if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
				t.Fatalf("link: %v", err)
			}
			moved, err := e.store.UpdateLead(ctx, lead, tc.patch)
			if err != nil {
				t.Fatal(err)
			}
			if moved.CompanyId != nil || moved.CompanyFacts != nil {
				t.Errorf("the lead kept company %v with facts %+v after its key stopped naming it", moved.CompanyId, moved.CompanyFacts)
			}
		})
	}
}

// Only a claim counts: a domain written on no company, or one the company
// released, leaves the offer to the name.
func TestOnlyAClaimedDomainMatchesALead(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	released := e.factCompany(ctx, t, CreateCompanyInput{
		DisplayName: "Kestrel Group", Domains: []CompanyDomainInput{{Domain: "kestrel.test", IsPrimary: true}},
	})
	if _, err := e.store.UpdateCompany(ctx, released, UpdateCompanyInput{Domains: &[]CompanyDomainInput{}}); err != nil {
		t.Fatalf("release the domain: %v", err)
	}
	byName := e.factCompany(ctx, t, CreateCompanyInput{DisplayName: "Kestrel Freight"})
	for _, key := range []string{"kestrel.test", "nobody-claims.test", "kestrel-freight"} {
		lead := e.factLead(ctx, t, e.owner, CreateLeadInput{
			Email: new(key + "@lead.test"), CompanyName: new("Kestrel Freight"), CandidateCompanyKey: new(key),
		})
		offered, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if offered.CompanyMatch == nil || ids.UUID(offered.CompanyMatch.Id) != byName.UUID ||
			offered.CompanyMatch.MatchedBy != crmcontracts.LeadCompanyMatchMatchedByName {
			t.Errorf("key %s: company_match = %+v, want the company the name names, matched by name", key, offered.CompanyMatch)
		}
	}
}

// The installation's own company is never offered by the domain it claims.
func TestTheAnchorCompanyIsNeverOfferedByDomain(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	anchor := e.factCompany(ctx, t, CreateCompanyInput{
		DisplayName: "Our Own GmbH", Domains: []CompanyDomainInput{{Domain: "our-own.test", IsPrimary: true}},
	})
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET is_anchor = true WHERE id = $1`, anchor)
		return err
	}); err != nil {
		t.Fatalf("make the company the installation's own: %v", err)
	}
	lead := e.factLead(ctx, t, e.owner, CreateLeadInput{
		Email: new("vera@our-own.test"), CompanyName: new("Someone Else"), CandidateCompanyKey: new("our-own.test"),
	})

	offered, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if offered.CompanyMatch != nil {
		t.Errorf("company_match = %+v, want no offer of the installation's own company", offered.CompanyMatch)
	}
}

func TestALeadListsUnderTheFactsOfItsLinkedCompanyElseItsOwnIndustry(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.factCompany(ctx, t, CreateCompanyInput{
		DisplayName: "Harbor Lines", Industry: new("logistics_transport"), SizeBand: new("51-200"),
		Address: &crmcontracts.Address{Country: new("DE")},
	})
	linked := e.factLead(ctx, t, e.owner, CreateLeadInput{
		Email: new("lou@harbor.test"), CompanyName: new("Harbor Lines"), Industry: new("Manufacturing"),
	})
	if _, err := e.store.LinkLeadCompany(ctx, linked, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	typed := e.factLead(ctx, t, e.owner, CreateLeadInput{Email: new("tia@typed.test"), Industry: new("manufacturing")})
	silent := e.factCompany(ctx, t, CreateCompanyInput{DisplayName: "Quiet Yard"})
	fallback := e.factLead(ctx, t, e.owner, CreateLeadInput{
		Email: new("ola@quiet.test"), CompanyName: new("Quiet Yard"), Industry: new("construction"),
	})
	if _, err := e.store.LinkLeadCompany(ctx, fallback, silent); err != nil {
		t.Fatalf("link: %v", err)
	}
	bare := e.factLead(ctx, t, e.owner, CreateLeadInput{Email: new("bo@bare.test")})

	read, err := e.store.GetLead(ctx, linked, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CompanyFacts == nil || deref((*string)(read.CompanyFacts.Industry)) != "logistics_transport" ||
		deref(read.CompanyFacts.Country) != "DE" || deref((*string)(read.CompanyFacts.SizeBand)) != "51-200" {
		t.Errorf("company_facts = %+v, want the linked company's industry, country and size band", read.CompanyFacts)
	}
	if read.Industry == nil || *read.Industry != crmcontracts.LeadIndustryManufacturing {
		t.Errorf("industry = %v, want the label the rep typed stored as its code", read.Industry)
	}
	for _, tc := range []struct {
		name    string
		filters LeadCompanyFactFilters
		want    []ids.UUID
	}{
		{"the company's industry", LeadCompanyFactFilters{Industry: new("logistics_transport")}, []ids.UUID{linked.UUID}},
		{"a typed industry the company overrides", LeadCompanyFactFilters{Industry: new("manufacturing")}, []ids.UUID{typed.UUID}},
		{"a typed industry beside a company with none", LeadCompanyFactFilters{Industry: new("construction")}, []ids.UUID{fallback.UUID}},
		{"the company's country, in either case", LeadCompanyFactFilters{Country: new("de")}, []ids.UUID{linked.UUID}},
		{"the company's size band", LeadCompanyFactFilters{SizeBand: new("51-200")}, []ids.UUID{linked.UUID}},
		{"a country no company is in", LeadCompanyFactFilters{Country: new("FR")}, []ids.UUID{}},
	} {
		got, err := e.listedUnder(ctx, t, tc.filters)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s listed %v, want %v (bare lead %s never)", tc.name, got, tc.want, bare)
		}
	}
}

func TestASeatWithoutCompanyReadKeepsTheTypedIndustryAndIsRefusedTheDials(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.factCompany(ctx, t, CreateCompanyInput{DisplayName: "Quarry Works", Industry: new("mining_metals")})
	lead := e.factLead(ctx, t, e.owner, CreateLeadInput{
		Email: new("ivo@quarry.test"), CompanyName: new("Quarry Works"), Industry: new("construction"),
	})
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	leadsOnly := e.linkerWith(e.owner, principal.RowScopeOwn, map[string]principal.ObjectGrant{"lead": {Read: true}})

	read, err := e.store.GetLead(leadsOnly, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CompanyFacts != nil || read.CompanyId != nil {
		t.Errorf("a seat without company read was shown company %v with facts %+v", read.CompanyId, read.CompanyFacts)
	}
	if read.Industry == nil || *read.Industry != crmcontracts.LeadIndustryConstruction {
		t.Errorf("industry = %v, want the lead's own construction", read.Industry)
	}
	for _, filters := range []LeadCompanyFactFilters{
		{Industry: new("construction")}, {Country: new("DE")}, {SizeBand: new("1-10")},
	} {
		if _, _, err := e.store.ListLeads(leadsOnly, ListLeadsInput{CompanyFacts: filters}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%+v for a seat without company read = %v, want ErrPermissionDenied", filters, err)
		}
	}
}

// A company the reader's row scope does not reach counts as no company. The
// read shows none of its facts, and no dial selects by them.
func TestACompanyOutsideTheRowScopeLendsTheLeadNoFacts(t *testing.T) {
	e := setupCapturePrivacy(t)
	ownerCtx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.factCompany(ownerCtx, t, CreateCompanyInput{
		DisplayName: "Silent Partners", Industry: new("financial_services"), Address: &crmcontracts.Address{Country: new("CH")},
	})
	teammate := e.linker(e.teammate, principal.RowScopeTeam)
	lead := e.factLead(teammate, t, e.teammate, CreateLeadInput{
		Email: new("max@silent.test"), CompanyName: new("Silent Partners"), Industry: new("insurance"),
	})
	if _, err := e.store.LinkLeadCompany(teammate, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := e.store.UpdateCompany(ownerCtx, company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("make the company private: %v", err)
	}

	read, err := e.store.GetLead(teammate, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CompanyFacts != nil || read.CompanyId != nil {
		t.Errorf("a company outside the row scope showed as %v with facts %+v", read.CompanyId, read.CompanyFacts)
	}
	for _, sort := range []*string{nil, new("created_at")} {
		rows, _, err := e.store.ListLeads(teammate, ListLeadsInput{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if ids.UUID(row.Id) == lead.UUID && (row.CompanyFacts != nil || row.CompanyId != nil) {
				t.Errorf("sort %v: the list row showed company %v with facts %+v", sort, row.CompanyId, row.CompanyFacts)
			}
		}
	}
	for _, tc := range []struct {
		filters LeadCompanyFactFilters
		want    []ids.UUID
	}{
		{LeadCompanyFactFilters{Industry: new("financial_services")}, []ids.UUID{}},
		{LeadCompanyFactFilters{Country: new("CH")}, []ids.UUID{}},
		{LeadCompanyFactFilters{Industry: new("insurance")}, []ids.UUID{lead.UUID}},
	} {
		got, err := e.listedUnder(teammate, t, tc.filters)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%+v listed %v, want %v", tc.filters, got, tc.want)
		}
	}
}

func TestQualifyingCarriesTheTypedIndustryOnlyToACompanyWithNone(t *testing.T) {
	e := setupCapturePrivacy(t)
	owner := e.linker(e.owner, principal.RowScopeOwn)
	readOnlyCompany := e.linkerWith(e.owner, principal.RowScopeOwn, map[string]principal.ObjectGrant{
		"contact":      {Create: true, Read: true, Update: true},
		"company":      {Read: true},
		"lead":         {Create: true, Read: true, Update: true},
		"relationship": {Create: true, Read: true, Update: true},
	})
	teammate := e.linker(e.teammate, principal.RowScopeTeam)
	teammateID := ids.From[ids.UserKind](e.teammate)
	for _, tc := range []struct {
		name     string
		promoter context.Context
		held     *string
		want     *string
		// outOfScope gives the company to the teammate, whom the promoter's own row scope does not reach.
		outOfScope bool
	}{
		{"a company with no industry", owner, nil, new("healthcare"), false},
		{"a company with one", owner, new("education"), new("education"), false},
		{"a promoter who may not change the company", readOnlyCompany, nil, nil, false},
		{"a company outside the promoter's row scope", owner, nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keeper, in := owner, CreateCompanyInput{DisplayName: "Clinic " + tc.name, Industry: tc.held}
			if tc.outOfScope {
				keeper, in.OwnerID = teammate, &teammateID
			}
			company := e.factCompany(keeper, t, in)
			lead := e.factLead(owner, t, e.owner, CreateLeadInput{
				Email: new(ids.NewV7().String() + "@clinic.test"), CompanyName: new("Clinic " + tc.name), Industry: new("Healthcare"),
			})
			if _, err := e.store.LinkLeadCompany(keeper, lead, company); err != nil {
				t.Fatalf("link: %v", err)
			}
			audits := e.industryAudits(owner, t, company)
			correlation := ids.NewV7()
			if _, err := e.store.QualifyLead(principal.WithCorrelationID(tc.promoter, correlation), lead,
				PromoteLeadInput{Trigger: string(TriggerHumanQualify)}); err != nil {
				t.Fatalf("promote: %v", err)
			}
			after, err := e.store.GetCompany(keeper, company, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if deref((*string)(after.Industry)) != deref(tc.want) {
				t.Errorf("the company's industry = %v, want %v", after.Industry, tc.want)
			}
			carried := tc.held == nil && tc.want != nil
			wrote := e.industryAudits(owner, t, company) - audits
			emitted := slices.Contains(e.eventTypesOf(owner, t, correlation), "company.updated")
			if (wrote == 1) != carried || wrote > 1 || emitted != carried {
				t.Errorf("the promotion wrote %d industry audit row(s) and company.updated %v, want them only when it carried the industry (%v)",
					wrote, emitted, carried)
			}
		})
	}
}

// industryAudits counts the company's audit rows that wrote its industry.
func (e *privacyEnv) industryAudits(ctx context.Context, t *testing.T, company ids.CompanyID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'company' AND entity_id = $1 AND after ? 'industry'`, company).Scan(&n)
	}); err != nil {
		t.Fatalf("count the company's industry audits: %v", err)
	}
	return n
}

// A lead's typed industry is written and cleared through the lead's own update
// path, which audits it like any other field.
func TestALeadsIndustryIsWrittenAndClearedThroughTheUpdatePath(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	lead := e.factLead(ctx, t, e.owner, CreateLeadInput{Email: new("rae@typed.test")})
	audits, _, _ := e.leadTrail(ctx, t, lead)

	set, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{Industry: new("Logistik")})
	if err != nil {
		t.Fatal(err)
	}
	if set.Industry == nil || *set.Industry != crmcontracts.LeadIndustryLogisticsTransport {
		t.Errorf("industry = %v, want logistics_transport", set.Industry)
	}
	if _, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{Industry: new("widgets")}); faultedField(err) != leadIndustryField {
		t.Errorf("text naming no industry = %v, want a refusal naming industry", err)
	}
	cleared, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{Clear: []string{"industry"}})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Industry != nil {
		t.Errorf("industry = %v after a clear, want none", *cleared.Industry)
	}
	if after, _, _ := e.leadTrail(ctx, t, lead); after != audits+2 {
		t.Errorf("the set and the clear wrote %d audit rows, want 2", after-audits)
	}
}

// A list narrows the default work queue as it narrows a sorted page.
func TestAListNarrowsTheLeadWorkQueue(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	member := e.factLead(ctx, t, e.owner, CreateLeadInput{Email: new("in@list.test")})
	e.factLead(ctx, t, e.owner, CreateLeadInput{Email: new("out@list.test")})
	membership := func(idColumn string, arg func(any) int) (string, error) {
		return storekit.SQLf("%s = $%d", idColumn, arg(member.UUID)), nil
	}
	for _, sort := range []*string{nil, new("created_at")} {
		leads, _, err := e.store.ListLeads(ctx, ListLeadsInput{Membership: membership, Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		if len(leads) != 1 || ids.UUID(leads[0].Id) != member.UUID {
			t.Errorf("sort %v listed %d lead(s), want only the list's member", sort, len(leads))
		}
	}
}
