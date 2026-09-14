// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What ACCEPTANCE does to the record, which is a different question from
// whether the read ran. The lifecycle suite next door proves the worker
// crawls, extracts, stages one proposal and records an honest outcome; these
// prove what the company becomes once a human answers that proposal.
//
// Three rules, each a place where a careless apply would look identical to a
// correct one on the dossier alone: offerings dedupe on their value key and
// never overwrite what a human already put there, an accepted employee_range
// fills the size band only where the mapping is unambiguous, and a rejection
// lands nothing at all.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two rules, and the read now holds both itself: a value_key duplicate is
// deduped before anything is written, and a fact a human already claimed is
// never overwritten. The second used to be the ACCEPT's job; the read applies
// directly now, so the guard has to live where the write does — and the human's
// row is seeded BEFORE the read for the same reason.
func TestDeepReadOfferingsDedupeOnValueKeyAndTheApplyRespectsHumanPrecedence(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")

	// A human has claimed the service fact. The read must land the product
	// beside it and leave this row exactly as it is.
	//
	// Seeded through the writer, because the row IS what this case protects:
	// CreateCompanyFact mints it human-owned from birth, which is the
	// property both enrichment upserts consult. The hand INSERT that stood here
	// made a row the writer cannot — it supplied value_key 'crm rollout' beside
	// the value "CRM Rollout (human curated)", which the writer's own derivation
	// keys as 'crm rollout (human curated)'. The precedence rule was therefore
	// being judged against a precondition no human could have created, and a
	// drift in captured_by, in source, or in that derivation would have left
	// this case green while production moved.
	humanService := contacts.FactCreateInput{Category: "offering", Field: "service", Value: "CRM Rollout"}
	if _, err := contacts.NewStore(e.DB()).CreateCompanyFact(
		e.As(e.Rep1, nil, integration.AdminPerms), ids.From[ids.CompanyKind](company), humanService,
	); err != nil {
		t.Fatalf("seeding the human-claimed service fact: %v", err)
	}

	done, _ := runServicesDeepRead(t, e, company)

	// The citation gate is binary (no model confidence), so a value_key
	// duplicate keeps its FIRST spelling — deterministic, page-ordered. The
	// dossier reports what the read evidenced, which is where the dedupe is
	// observable now that nothing is staged.
	if len(done.Facts) != 2 {
		t.Fatalf("read facts = %+v, want the deduped service + the product", done.Facts)
	}
	service := done.Facts[0]
	if service.Field != "service" || service.ValueKey != "crm rollout" || service.Value != "CRM Rollout — implementation projects" {
		t.Fatalf("read service = %+v, want the first-seen spelling under value_key 'crm rollout'", service)
	}

	var factRows int
	var serviceValue, serviceCapturedBy, productValue string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM company_fact WHERE company_id = $1`, company).Scan(&factRows); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`SELECT value, captured_by FROM company_fact
			 WHERE company_id = $1 AND field = 'service' AND value_key = 'crm rollout'`,
			company).Scan(&serviceValue, &serviceCapturedBy); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT coalesce(max(value), '') FROM company_fact
			 WHERE company_id = $1 AND field = 'product'`, company).Scan(&productValue)
	}); err != nil {
		t.Fatal(err)
	}
	if factRows != 2 {
		t.Fatalf("%d company_fact rows after the read, want 2 (the human's service + the landed product)", factRows)
	}
	if serviceValue != humanService.Value || serviceCapturedBy != "human:"+e.Rep1.String() {
		t.Fatalf("service row = %q by %q — the read overwrote a human-claimed fact", serviceValue, serviceCapturedBy)
	}
	if productValue != "Margince — our CRM product" {
		t.Fatalf("product row = %q, want the read's product landed beside the human's row", productValue)
	}
}

func TestAcceptedEmployeeRangeFactFillsSizeBandWhenUnambiguous(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	employeeRangeFact := func(value string) []contacts.DeepReadFact {
		return []contacts.DeepReadFact{{
			Category: "company", Field: "employee_range", Value: value,
			EvidenceSnippet: "our team of " + value, SourceURL: "https://acme.example/about", Confidence: 0.9,
		}}
	}
	readSizeBand := func(company ids.UUID) *string {
		var sizeBand *string
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT size_band FROM company WHERE id = $1`, company).Scan(&sizeBand)
		}); err != nil {
			t.Fatalf("reading size_band: %v", err)
		}
		return sizeBand
	}

	// A cleanly-phrased range fills the chip's column on accept.
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	if err := store.ApplyDeepRead(ctx, contacts.DeepReadProposal{
		CompanyID: ids.From[ids.CompanyKind](company),
		SourceURL: "https://acme.example",
		Facts:     employeeRangeFact("25 to 50"),
	}); err != nil {
		t.Fatalf("ApplyDeepRead: %v", err)
	}
	if got := readSizeBand(company); got == nil || *got != "11-50" {
		t.Fatalf("size_band after accept = %v, want 11-50", got)
	}

	// A later read never overwrites the standing value — fill-once.
	if err := store.ApplyDeepRead(ctx, contacts.DeepReadProposal{
		CompanyID: ids.From[ids.CompanyKind](company),
		SourceURL: "https://acme.example",
		Facts:     employeeRangeFact("about 300 contacts"),
	}); err != nil {
		t.Fatalf("second ApplyDeepRead: %v", err)
	}
	if got := readSizeBand(company); got == nil || *got != "11-50" {
		t.Fatalf("size_band after re-accept = %v, want the first fill kept", got)
	}

	// A range spanning two bands abstains: the fact lands as evidence, the
	// column stays empty rather than holding a guess.
	vague := insertCompany(t, e, e.Rep1, "vague.example", "")
	if err := store.ApplyDeepRead(ctx, contacts.DeepReadProposal{
		CompanyID: ids.From[ids.CompanyKind](vague),
		SourceURL: "https://vague.example",
		Facts:     employeeRangeFact("50-200 employees"),
	}); err != nil {
		t.Fatalf("ambiguous ApplyDeepRead: %v", err)
	}
	if got := readSizeBand(vague); got != nil {
		t.Fatalf("an ambiguous range filled size_band = %q, want NULL", *got)
	}
	var factValue string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT value FROM company_fact
			  WHERE company_id = $1 AND field = 'employee_range'`, vague).Scan(&factValue)
	}); err != nil {
		t.Fatalf("reading the fact row: %v", err)
	}
	if factValue != "50-200 employees" {
		t.Fatalf("fact row = %q, want the raw stated range kept as evidence", factValue)
	}

	// A human-claimed employee_range fact blocks the whole promotion: the
	// upsert refuses the agent's fact, so the column must not contradict the
	// human's standing statement either.
	claimed := insertCompany(t, e, e.Rep1, "claimed.example", "")
	// Through the writer, for the reason the offerings case states at length:
	// the standing statement this promotion must not contradict is a row
	// CreateCompanyFact makes, and a fixture that merely resembles one
	// keeps passing after the writer's shape has moved.
	if _, err := store.CreateCompanyFact(ctx, ids.From[ids.CompanyKind](claimed),
		contacts.FactCreateInput{Category: "company", Field: "employee_range", Value: "11-50"},
	); err != nil {
		t.Fatalf("seeding the human-claimed employee_range fact: %v", err)
	}
	if err := store.ApplyDeepRead(ctx, contacts.DeepReadProposal{
		CompanyID: ids.From[ids.CompanyKind](claimed),
		SourceURL: "https://claimed.example",
		Facts:     employeeRangeFact("about 300 contacts"),
	}); err != nil {
		t.Fatalf("ApplyDeepRead against a human-claimed fact: %v", err)
	}
	if got := readSizeBand(claimed); got != nil {
		t.Fatalf("a refused fact still promoted size_band = %q, want NULL", *got)
	}
}

// A rejection of a proposal staged BEFORE reads began applying directly. The
// read is not run here: it would land its own findings and there would be
// nothing left for a rejection to be observed against.
func TestDeepReadRejectionLandsNothing(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	_, svc := newDeepReadTestWorker(e, acmeServicesSite(), servicesDeepBrain())
	read, _ := startDeepRead(t, e, company)
	proposal := stageLegacyDeepReadProposal(t, e, svc, company, read.ID, nil, servicesOfferings())

	if _, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), ids.From[ids.ApprovalKind](proposal), false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_fact`); n != 0 {
		t.Fatalf("%d company_fact rows after a rejection, want 0", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_profile_field`); n != 0 {
		t.Fatalf("%d profile-field rows after a rejection, want 0", n)
	}
}

// fakeInserter stands in for the insert-only River client so handler
// tests can count what start enqueues, and what opts it queued it under.
type fakeInserter struct {
	inserts []river.JobArgs
	opts    []*river.InsertOpts
	err     error
}

func (f *fakeInserter) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error {
	if f.err != nil {
		return f.err
	}
	f.inserts = append(f.inserts, args)
	f.opts = append(f.opts, opts)
	return nil
}

func newDeepReadTestEngine(e *integration.Env, inserter *fakeInserter) *deepReadEngine {
	return &deepReadEngine{
		contacts: e.Contacts,
		enqueue:  inserter,
	}
}

// postDeepRead drives the start handler as the given caller and decodes
// the 202 handle (or fails the test on any other status when want202).
func postDeepRead(t *testing.T, e *integration.Env, engine *deepReadEngine, caller ids.UUID, company ids.UUID) (*httptest.ResponseRecorder, crmcontracts.SiteReadStarted) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/companies/"+company.String()+"/deep-read", nil).
		WithContext(e.As(caller, nil, integration.AdminPerms))
	rec := httptest.NewRecorder()
	engine.start(rec, req, openapi_types.UUID(company))
	var started crmcontracts.SiteReadStarted
	if rec.Code == http.StatusAccepted {
		if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
			t.Fatalf("decoding SiteReadStarted: %v", err)
		}
	}
	return rec, started
}

// servicesOfferings is what the services fixture evidences: one service and
// one product, already deduped on their value keys.
func servicesOfferings() []contacts.DeepReadFact {
	return []contacts.DeepReadFact{
		{
			Category: "offering", Field: "service",
			Value: "CRM Rollout — implementation projects", ValueKey: "crm rollout",
			EvidenceSnippet: "We deliver CRM Rollout projects end to end.",
			SourceURL:       seedURL + "/services", Confidence: 1,
		},
		{
			Category: "offering", Field: "product",
			Value: "Margince — our CRM product", ValueKey: "margince",
			EvidenceSnippet: "Margince is our CRM product.",
			SourceURL:       seedURL + "/services", Confidence: 1,
		},
	}
}
