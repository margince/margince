// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The installation's own company: the anchor pointer (0083) is what makes
// "has this installation described itself yet?" answerable, the form's save is
// the human's confirm-first write, and a value a human has saved is theirs —
// a later agent read-back of the same site leaves it alone.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func strptr(s string) *string { return &s }

func TestCompanyIsUnsetUntilAHumanSavesIt(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	// A freshly bootstrapped installation (ADR-0061) has a company row
	// for nobody: the anchor is unset, and that IS the onboarding signal.
	if _, err := store.GetAnchorCompany(ctx); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("GetAnchorCompany on a bare installation → %v, want ErrNotFound", err)
	}

	saved, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Website:     strptr("https://www.acme.example/about"),
		Fields: map[string]*string{
			"legal_name":    strptr("Acme Gesellschaft mit beschränkter Haftung"),
			"offer_summary": strptr("Revenue operations software"),
			"icp":           strptr("RevOps at SaaS scale-ups"),
			// A field nobody filled stays absent rather than becoming "".
			"usp": nil,
		},
	})
	if err != nil {
		t.Fatalf("SaveCompany: %v", err)
	}
	if saved.DisplayName != "Acme GmbH" {
		t.Fatalf("saved name = %q", saved.DisplayName)
	}
	if !saved.MinimumComplete {
		t.Fatal("the three semantic fields did not make the company minimum-complete")
	}
	// The website is stored as the bare domain — the same handle a read-back
	// resolves companies by — so a full URL normalises on the way in.
	if saved.Website == nil || *saved.Website != "acme.example" {
		t.Fatalf("saved website = %v, want acme.example", saved.Website)
	}
	if _, filled := saved.Fields["usp"]; filled {
		t.Fatalf("an unsent field was written: %+v", saved.Fields)
	}

	// The mark is what makes the company findable; without it the row is just
	// another company.
	var anchors int
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM company
			  WHERE id = $1 AND is_anchor AND archived_at IS NULL`,
			saved.CompanyID).Scan(&anchors)
	})
	if err != nil {
		t.Fatal(err)
	}
	if anchors != 1 {
		t.Fatalf("the saved company is not marked as the installation's own (%d anchor rows)", anchors)
	}
	if audits := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1 AND action = 'create'`,
		saved.CompanyID.UUID); audits != 1 {
		t.Fatalf("company save wrote %d create audits, want 1", audits)
	}
	if outbox := e.WsCount(t,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'company.created' AND envelope#>>'{entity,id}' = $1`,
		saved.CompanyID.String()); outbox != 1 {
		t.Fatalf("company save wrote %d company.created events, want 1", outbox)
	}

	// Re-reading is the form's own round-trip.
	got, err := store.GetAnchorCompany(ctx)
	if err != nil {
		t.Fatalf("GetAnchorCompany after save: %v", err)
	}
	if got.CompanyID != saved.CompanyID || got.Fields["icp"] != "RevOps at SaaS scale-ups" {
		t.Fatalf("GetAnchorCompany = %+v, want the saved company", got)
	}

	// A second save updates the anchor rather than minting a rival company.
	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme SE",
		Fields:      map[string]*string{"icp": strptr("RevOps at enterprise")},
	}); err != nil {
		t.Fatalf("second SaveCompany: %v", err)
	}
	var companies int
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM company`).Scan(&companies)
	})
	if err != nil {
		t.Fatal(err)
	}
	if companies != 1 {
		t.Fatalf("saving twice created %d companies, want the one anchor", companies)
	}

	// A field sent empty is cleared, not stored as the empty answer.
	cleared, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme SE",
		Fields:      map[string]*string{"icp": strptr("")},
	})
	if err != nil {
		t.Fatalf("clearing SaveCompany: %v", err)
	}
	if _, filled := cleared.Fields["icp"]; filled {
		t.Fatalf("cleared field is still present: %+v", cleared.Fields)
	}
}

// Editing the website has to actually move the primary domain. A company
// has at most one (uq_company_domain_primary), so a naive insert collides with the
// old one — and a swallowed collision means the human changed their website,
// saw a 200, and kept the old site.
func TestCompanyWebsiteCanBeChangedAfterTheFirstSave(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	base := contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Fields: map[string]*string{
			"legal_name": strptr("Acme GmbH"), "registered_address": strptr("Berlin"),
			"register_vat": strptr("DE123"), "industry": strptr("Software"),
		},
	}
	first := base
	first.Website = strptr("https://old.example")
	if _, err := store.SaveCompany(ctx, first); err != nil {
		t.Fatalf("first SaveCompany: %v", err)
	}

	moved := base
	moved.Website = strptr("https://new.example")
	got, err := store.SaveCompany(ctx, moved)
	if err != nil {
		t.Fatalf("changing the website: %v", err)
	}
	if got.Website == nil {
		t.Fatal("the saved company has no website at all after the change")
	}
	if *got.Website != "new.example" {
		t.Fatalf("the saved website is %q, want new.example — the edit was lost", *got.Website)
	}

	// Exactly one primary, and it is the new site: the old row must be demoted,
	// not left alongside as a rival primary.
	var primaries int
	var primary string
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM company_domain
			  WHERE company_id = $1 AND is_primary AND archived_at IS NULL`,
			got.CompanyID).Scan(&primaries); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT domain FROM company_domain
			  WHERE company_id = $1 AND is_primary AND archived_at IS NULL`,
			got.CompanyID).Scan(&primary)
	})
	if err != nil {
		t.Fatal(err)
	}
	if primaries != 1 || primary != "new.example" {
		t.Fatalf("primary domains = %d (%q), want exactly 1 as new.example", primaries, primary)
	}

	// Re-saving the SAME site is idempotent, not a conflict with itself.
	if _, err := store.SaveCompany(ctx, moved); err != nil {
		t.Fatalf("re-saving the same website: %v", err)
	}
}

func TestCompanySavedByAHumanSurvivesALaterReadBack(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	human := e.As(e.Rep1, nil, integration.AdminPerms)

	saved, err := store.SaveCompany(human, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Website:     strptr("https://acme.example"),
		Fields:      map[string]*string{"icp": strptr("What the human says we sell to")},
	})
	if err != nil {
		t.Fatalf("SaveCompany: %v", err)
	}

	// The human's own words carry human provenance — which is exactly what
	// the read-back's upsert refuses to overwrite.
	var capturedBy, source string
	err = database.WithWorkspaceTx(human, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT captured_by, source FROM company_profile_field
			  WHERE company_id = $1 AND field = 'icp'`, saved.CompanyID).Scan(&capturedBy, &source)
	})
	if err != nil {
		t.Fatal(err)
	}
	if capturedBy != "human:"+e.Rep1.String() || source != "human" {
		t.Fatalf("human-typed field stamped captured_by=%q source=%q, want human:<id> / human", capturedBy, source)
	}

	// Now an agent reads the same site back and its accept lands on the same
	// company (resolved by the domain the form recorded).
	agent := principal.WithActor(human, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:coldstart",
		UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms,
	})
	companyID, err := store.ApplyColdStartProfile(agent, contacts.ApplyColdStartProfileInput{
		SourceURL: "https://acme.example",
		Fields: []contacts.ColdStartFieldInput{{
			Field: "icp", Value: "What the website says", EvidenceSnippet: "Built for RevOps",
			SourceURL: "https://acme.example", Confidence: 0.9,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyColdStartProfile: %v", err)
	}
	if companyID != saved.CompanyID {
		t.Fatalf("the read-back landed on %s, not the anchor %s — the form's domain should resolve to the company",
			companyID, saved.CompanyID)
	}

	got, err := store.GetAnchorCompany(human)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields["icp"] != "What the human says we sell to" {
		t.Fatalf("an agent read-back overwrote the human's own value: %q", got.Fields["icp"])
	}
}

func TestFormResaveDoesNotClobberAHeaderDescriptionEdit(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	// The first form save fills the empty header line from the summary.
	saved, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Website:     strptr("https://acme.example"),
		Fields:      map[string]*string{"offer_summary": strptr("Revenue operations software")},
	})
	if err != nil {
		t.Fatalf("SaveCompany: %v", err)
	}
	readDescription := func() *string {
		var description *string
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT description FROM company WHERE id = $1`, saved.CompanyID).Scan(&description)
		}); err != nil {
			t.Fatalf("reading description: %v", err)
		}
		return description
	}
	if got := readDescription(); got == nil || *got != "Revenue operations software" {
		t.Fatalf("description after the first form save = %v, want the typed summary", got)
	}

	// The header's inline edit is the one editor of a standing value.
	if _, err := store.UpdateCompany(ctx, saved.CompanyID, contacts.UpdateCompanyInput{
		Description: strptr("The RevOps platform for manufacturers"),
	}); err != nil {
		t.Fatalf("UpdateCompany: %v", err)
	}

	// A later form save re-sends the unchanged summary; the newer header line
	// must survive it.
	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Fields:      map[string]*string{"offer_summary": strptr("Revenue operations software")},
	}); err != nil {
		t.Fatalf("second SaveCompany: %v", err)
	}
	if got := readDescription(); got == nil || *got != "The RevOps platform for manufacturers" {
		t.Fatalf("description after a form resave = %v, want the header edit kept", got)
	}
}

func TestAcceptedOfferSummaryWritesTheDescriptionColumn(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	base := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	agent := principal.WithActor(base, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:coldstart",
		UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms,
	})

	// The header renders company.description; an accepted offer_summary is
	// the one-sentence answer, so the apply writes the column.
	companyID, err := store.ApplyColdStartProfile(agent, contacts.ApplyColdStartProfileInput{
		SourceURL: "https://summarized.example",
		Fields: []contacts.ColdStartFieldInput{{
			Field: "offer_summary", Value: "Revenue operations software for mid-market manufacturers",
			EvidenceSnippet: "We build RevOps software", SourceURL: "https://summarized.example", Confidence: 0.9,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyColdStartProfile: %v", err)
	}

	readDescription := func() *string {
		var description *string
		if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT description FROM company WHERE id = $1`, companyID).Scan(&description)
		}); err != nil {
			t.Fatalf("reading description: %v", err)
		}
		return description
	}
	got := readDescription()
	if got == nil || *got != "Revenue operations software for mid-market manufacturers" {
		t.Fatalf("description after accept = %v, want the accepted offer_summary", got)
	}

	// A later accept carries the site's newer copy onto the column as well as
	// onto the evidence row. Nobody typed the standing value — one automated
	// read replaces another, which is what lets a re-crawl correct a summary
	// that has gone stale, or one an agent wrote from a meeting transcript.
	// A value a CONTACT authored is held instead, and the form-resave case
	// above is the assertion that keeps that half honest.
	if _, err := store.ApplyColdStartProfile(agent, contacts.ApplyColdStartProfileInput{
		SourceURL: "https://summarized.example",
		Fields: []contacts.ColdStartFieldInput{{
			Field: "offer_summary", Value: "A different sentence entirely",
			EvidenceSnippet: "New copy", SourceURL: "https://summarized.example", Confidence: 0.9,
		}},
	}); err != nil {
		t.Fatalf("second ApplyColdStartProfile: %v", err)
	}
	if got := readDescription(); got == nil || *got != "A different sentence entirely" {
		t.Fatalf("description after re-accept = %v, want the site's newer sentence", got)
	}
	var evidenceValue string
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT value FROM company_profile_field
			  WHERE company_id = $1 AND field = 'offer_summary'`, companyID).Scan(&evidenceValue)
	}); err != nil {
		t.Fatalf("reading the evidence row: %v", err)
	}
	if evidenceValue != "A different sentence entirely" {
		t.Fatalf("evidence row after re-accept = %q, want the refreshed value", evidenceValue)
	}
}

// What an overlong summary does to the header line it renders into.
//
// `offer_summary` is what an installation says about what it sells and the
// contract takes 2000 characters of it; `company_description_length` caps
// the header at 500. The header is one RENDERING of the summary, so a value too
// long for the line is a value to shorten — not a write to drop, which is what
// used to happen: the summary saved, the header stayed blank, and nothing said
// why to the caller, the audit trail or the operator.
//
// Both shapes, because the cut is a rule and not just a length. Spelled in
// multibyte characters so a byte-counting cut fails this test: the CHECK counts
// characters, and a byte cut would refuse text the column accepts.
func TestAnOverlongOfferSummaryFillsTheHeaderWithItsFirstLine(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	base := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	agent := principal.WithActor(base, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:coldstart",
		UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms,
	})

	apply := func(t *testing.T, source, summary string) (*string, string) {
		t.Helper()
		companyID, err := store.ApplyColdStartProfile(agent, contacts.ApplyColdStartProfileInput{
			SourceURL: source,
			Fields: []contacts.ColdStartFieldInput{{
				Field: "offer_summary", Value: summary,
				EvidenceSnippet: "We build RevOps software", SourceURL: source, Confidence: 0.9,
			}},
		})
		if err != nil {
			t.Fatalf("ApplyColdStartProfile: %v", err)
		}
		var description *string
		var evidenceValue string
		if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
			if err := tx.QueryRow(context.Background(),
				`SELECT description FROM company WHERE id = $1`, companyID).Scan(&description); err != nil {
				return err
			}
			return tx.QueryRow(context.Background(),
				`SELECT value FROM company_profile_field
				  WHERE company_id = $1 AND field = 'offer_summary'`, companyID).Scan(&evidenceValue)
		}); err != nil {
			t.Fatalf("reading the apply's result: %v", err)
		}
		return description, evidenceValue
	}

	// One 501-character word: there is no boundary to cut at, and a bounded
	// value still beats an empty one.
	unbroken := strings.Repeat("ü", 501)
	description, evidenceValue := apply(t, "https://unbroken.example", unbroken)
	if description == nil {
		t.Fatal("a 501-character summary left the header NULL — the fill it is too long for is the one a reader sees, and dropping it is the silence this replaced")
	}
	if got := utf8.RuneCountInString(*description); got != 500 {
		t.Errorf("the header line is %d characters; want the 500 the column admits", got)
	}
	if evidenceValue != unbroken {
		t.Error("the evidence row lost the accepted summary — the header is a rendering of it, and shortening the rendering may not shorten the record")
	}

	// The same length in words, chosen so the hard 500-rune cut lands INSIDE a
	// word — a phrase whose repeat happens to end on 500 would pass whether the
	// boundary rule ran or not. The assertion is then the RULE rather than a
	// second copy of the arithmetic: the line stops on a boundary the summary
	// itself has, so no word is cut in half.
	spoken := strings.Repeat("Wörter ", 100)
	description, _ = apply(t, "https://spoken.example", spoken)
	if description == nil {
		t.Fatal("a long spoken summary left the header NULL")
	}
	line := *description
	if count := utf8.RuneCountInString(line); count > 500 || count == 0 {
		t.Errorf("the header line is %d characters; want a non-empty line within the 500 the column admits", count)
	}
	rest, isPrefix := strings.CutPrefix(spoken, line)
	if !isPrefix {
		t.Fatalf("the header line %q is not the start of the summary it renders", line)
	}
	if next, _ := utf8.DecodeRuneInString(rest); next != ' ' {
		t.Errorf("the line stops mid-word, before %q — a word cut in half reads as a bug where a clean stop reads as a summary", rest[:min(20, len(rest))])
	}

	// At the cap the value passes through untouched: the shortening is for what
	// does not fit, and a summary that fits is not a summary to edit.
	atCap := strings.Repeat("ü", 500)
	description, _ = apply(t, "https://atcap.example", atCap)
	if description == nil || *description != atCap {
		t.Error("a summary of exactly 500 characters should reach the header unchanged")
	}
}

func TestColdStartCreateWithoutLegalNameUsesDerivedDomainName(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	base := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	agent := principal.WithActor(base, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:coldstart",
		UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms,
	})

	// A fresh domain with no existing company and no accepted legal_name: the create
	// path must name the company from the domain's registrable label ("Docusign",
	// not "eu.docusign.net") and mark it name_source='domain' so a later richer
	// source may overwrite it (ADR-0072/A118).
	companyID, err := store.ApplyColdStartProfile(agent, contacts.ApplyColdStartProfileInput{
		SourceURL: "https://eu.docusign.net",
		Fields: []contacts.ColdStartFieldInput{{
			Field: "icp", Value: "eSignature buyers", EvidenceSnippet: "For every agreement",
			SourceURL: "https://eu.docusign.net", Confidence: 0.9,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyColdStartProfile: %v", err)
	}

	var displayName, nameSource string
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT display_name, name_source FROM company WHERE id = $1`, companyID).Scan(&displayName, &nameSource)
	}); err != nil {
		t.Fatalf("reading the created company: %v", err)
	}
	if displayName != "Docusign" || nameSource != "domain" {
		t.Fatalf("created company = (%q, %q), want (Docusign, domain)", displayName, nameSource)
	}
}

func TestCompanyContextIsScopedProvenanceBearingAndChangesWithTheProfile(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	saved, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Website:     strptr("https://acme.example"),
		Fields: map[string]*string{
			"offer_summary": strptr("Revenue operations software"),
			"icp":           strptr("Mid-market manufacturers"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `
		INSERT INTO company_fact (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'offering', 'service', 'CRM rollout', 'crm rollout',
		        '', '', 1, 'human', $2)`, saved.CompanyID, "human:"+e.Rep1.String())

	// The cross-tenant arm is gone with the mechanism it tested. It seeded a
	// second workspace with its own ANCHOR company and asserted this
	// tenant's context read did not reach it — and `uq_company_anchor` is
	// installation-wide since ADR-0091 §8 phase B, so a second anchor cannot
	// exist to be reached. What is left below is what the read is FOR: the
	// scopes it assembles, the provenance it carries, and the fingerprint that
	// moves only when the profile does.

	first, err := store.GetCompanyContext(ctx, []contacts.CompanyContextScope{
		contacts.CompanyContextOffer, contacts.CompanyContextPositioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Scopes) != 2 || first.Scopes[0].Scope != contacts.CompanyContextPositioning || first.Scopes[1].Scope != contacts.CompanyContextOffer {
		t.Fatalf("context scopes = %#v, want canonical positioning then offer", first.Scopes)
	}
	if len(first.Scopes[0].Items) != 1 || first.Scopes[0].Items[0].Key != "icp" || first.Scopes[0].Items[0].Source != "human" {
		t.Fatalf("positioning context = %#v, want human-provenance ICP", first.Scopes[0].Items)
	}
	if len(first.Scopes[1].Items) != 2 {
		t.Fatalf("offer context = %#v, want summary and repeatable service", first.Scopes[1].Items)
	}
	again, err := store.GetCompanyContext(ctx, []contacts.CompanyContextScope{
		contacts.CompanyContextPositioning, contacts.CompanyContextOffer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != again.Fingerprint {
		t.Fatalf("unchanged context fingerprint moved from %q to %q", first.Fingerprint, again.Fingerprint)
	}

	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: saved.DisplayName,
		Fields: map[string]*string{
			"offer_summary": strptr("Revenue intelligence software"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := store.GetCompanyContext(ctx, []contacts.CompanyContextScope{
		contacts.CompanyContextOffer, contacts.CompanyContextPositioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == changed.Fingerprint {
		t.Fatal("editing a contributing profile value did not change the context fingerprint")
	}
}

// A fact with no confidence at all is not a broken company read. The column
// allows one and the technical-signal lane writes exactly that — it observes a
// mail provider from a DNS record, which is a fact it either read or did not,
// with no score to give it. A read that scanned that row into a plain float
// answered the whole profile with an error, which takes down the onboarding
// gate and the shell's brand block for a company that is perfectly fine.
func TestTheCompanyReadSurvivesAFactNobodyScored(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	store := contacts.NewStore(e.DB())

	saved, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme GmbH",
		Fields: map[string]*string{
			"offer_summary": strptr("Revenue operations software"),
			"icp":           strptr("RevOps at SaaS scale-ups"),
		},
	})
	if err != nil {
		t.Fatalf("save the company: %v", err)
	}
	// Written through the real lane, not planted: what makes this row reachable
	// is that a production writer records no confidence for it.
	if err := store.ApplyTechnicalEnrichment(ctx, contacts.TechnicalEnrichment{
		CompanyID: saved.CompanyID,
		Completed: []contacts.TechnicalLane{contacts.LaneDNS},
		Observations: []contacts.TechnicalObservation{{
			Field: "mail_provider", ValueKey: "google", Value: "Google Workspace",
			Evidence: "aspmx.l.google.com", SourceURL: "dns:acme.example",
		}},
		ObservedAt: time.Now(),
	}, nil); err != nil {
		t.Fatalf("record the technical signal: %v", err)
	}

	company, err := store.GetAnchorCompany(ctx)
	if err != nil {
		t.Fatalf("read the company back: %v", err)
	}
	var unscored int
	for _, fact := range company.Facts {
		if fact.Confidence == nil {
			unscored++
		}
	}
	if unscored != 1 {
		t.Fatalf("%d facts came back unscored, want the one the lane wrote — a zero here would "+
			"read as a fact scored worthless rather than one nobody scored", unscored)
	}
}
