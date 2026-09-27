// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// search_report_evidence as a client reaches it: through the registered tool,
// over a run saved by the real analytics engine, searched by the real search
// module. The run is the cohort: a record outside it is never evidence however
// well it matches, and a share of the cohort is stated only when every record
// of it was reached and judged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// evidenceFixture is two cells of activities-by-kind plus a matching record
// outside the population. Seven calls clear the floor of five: four mention
// pricing and three do not. Six notes, one mentioning pricing, are the other
// cell; the deal is not an activity at all.
type evidenceFixture struct {
	pricingCalls, otherCalls []ids.UUID
	pricingNote              ids.UUID
	pricingDeal              ids.UUID
}

func seedEvidenceFixture(t *testing.T, e *SearchEnv) evidenceFixture {
	t.Helper()
	activity := func(kind, body string) ids.UUID {
		return e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			VALUES ($1, $2, 'Weekly check-in', $3, now() - interval '1 hour', 'manual', 'human:x')`, kind, body)
	}
	var f evidenceFixture
	for range 4 {
		f.pricingCalls = append(f.pricingCalls, activity("call", "The buyer pushed back on pricing again"))
	}
	for range 3 {
		f.otherCalls = append(f.otherCalls, activity("call", "Walked through the onboarding plan"))
	}
	f.pricingNote = activity("note", "Pricing sheet sent over")
	for range 5 {
		activity("note", "Filed the meeting minutes")
	}
	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default) VALUES ($1, 'Sales', true)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Open', 1, 'open', 20)`, pipeline)
	f.pricingDeal = e.SeedID(t, `INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status, source, captured_by)
		VALUES ($1, $2, $3, 'Pricing review', $4, 'open', 'manual', 'human:x')`, pipeline, stage, e.Rep1)
	return f
}

// evidenceReader is Rep1 with the grants a report and its records need.
func (e *SearchEnv) evidenceReader() context.Context {
	grants := searchReadGrants()
	grants["forecast"] = principal.ObjectGrant{Read: true}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs:     []ids.UUID{e.Team1},
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

// saveActivitiesByKind saves the grouped count the way run_analytics_query does.
func saveActivitiesByKind(ctx context.Context, t *testing.T, e *SearchEnv) ids.UUID {
	t.Helper()
	q := analyticsquery.Query{
		Entity:   "activities-by-kind",
		GroupBy:  []string{"kind"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "activities"}},
	}
	var runID ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		answer, err := compose.RunAnalyticsQuery(ctx, tx, q, analyticsquery.DefaultFloor)
		if err != nil {
			return err
		}
		runID, err = compose.SaveReportRun(ctx, tx, q, answer, analyticsquery.DefaultFloor)
		return err
	}); err != nil {
		t.Fatalf("saving the run: %v", err)
	}
	return runID
}

func searchEvidence(ctx context.Context, e *SearchEnv, args string) (agents.SearchReportEvidenceResult, error) {
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	out, err := registry.Invoke(ctx, "search_report_evidence", json.RawMessage(args))
	if err != nil {
		return agents.SearchReportEvidenceResult{}, err
	}
	spec, _ := registry.Spec("search_report_evidence")
	if defect := agents.ResultDefect(spec.OutputSchema, out); defect != "" {
		return agents.SearchReportEvidenceResult{}, fmt.Errorf("the answer breaks its own schema: %s", defect)
	}
	var sealed sealedResult
	if err := json.Unmarshal(out, &sealed); err != nil {
		return agents.SearchReportEvidenceResult{}, fmt.Errorf("the result is not an envelope: %w (%s)", err, out)
	}
	var answer agents.SearchReportEvidenceResult
	if err := json.Unmarshal(sealed.Data, &answer); err != nil {
		return agents.SearchReportEvidenceResult{}, fmt.Errorf("unreadable payload %s: %w", sealed.Data, err)
	}
	return answer, nil
}

func mustSearchEvidence(ctx context.Context, t *testing.T, e *SearchEnv, args string) agents.SearchReportEvidenceResult {
	t.Helper()
	answer, err := searchEvidence(ctx, e, args)
	if err != nil {
		t.Fatalf("search_report_evidence %s\n  → %v", args, err)
	}
	return answer
}

func citedIDs(answer agents.SearchReportEvidenceResult) map[ids.UUID]bool {
	out := map[ids.UUID]bool{}
	for _, hit := range answer.Citations {
		out[hit.Record.ID] = true
	}
	return out
}

func evidenceNoted(answer agents.SearchReportEvidenceResult, code string) bool {
	for _, note := range answer.Notes {
		if note.Code == code {
			return true
		}
	}
	return false
}

// One cell searched whole: its matches are cited, its non-matches are
// counterexamples, and the share is stated because nothing was out of reach.
// The note and the deal also say "pricing" and are never cited.
func TestReportEvidenceSearchesOnlyTheCellsRecords(t *testing.T) {
	e := SetupSearch(t)
	f := seedEvidenceFixture(t, e)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	answer := mustSearchEvidence(ctx, t, e,
		`{"run_id":"`+runID.String()+`","cell":["call"],"query":"pricing"}`)

	cited := citedIDs(answer)
	for _, id := range f.pricingCalls {
		if !cited[id] {
			t.Errorf("the call %s mentions pricing and is not cited: %+v", id, answer.Citations)
		}
	}
	if cited[f.pricingNote] || cited[f.pricingDeal] {
		t.Fatalf("a record outside the cell was cited as its evidence: %+v", answer.Citations)
	}
	if len(answer.Citations) != len(f.pricingCalls) {
		t.Errorf("%d citations, want the %d pricing calls", len(answer.Citations), len(f.pricingCalls))
	}
	countered := map[ids.UUID]bool{}
	for _, record := range answer.Counterexamples {
		countered[record.ID] = true
	}
	for _, id := range f.otherCalls {
		if !countered[id] {
			t.Errorf("the call %s does not mention pricing and is not a counterexample", id)
		}
	}
	if answer.Coverage != agents.CoverageCompleteExact {
		t.Fatalf("coverage = %q with every record reached and judged, notes %+v", answer.Coverage, answer.Notes)
	}
	if answer.Prevalence == nil || answer.Prevalence.Matched != 4 || answer.Prevalence.Of != 7 {
		t.Errorf("prevalence = %+v, want 4 of 7", answer.Prevalence)
	}
}

// Without a cell the search covers every record the run measured, so the
// note's mention of pricing becomes evidence; the cell above narrowed it away.
func TestReportEvidenceWithoutACellSearchesTheWholeRun(t *testing.T) {
	e := SetupSearch(t)
	f := seedEvidenceFixture(t, e)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	answer := mustSearchEvidence(ctx, t, e, `{"run_id":"`+runID.String()+`","query":"pricing"}`)

	cited := citedIDs(answer)
	if !cited[f.pricingNote] {
		t.Errorf("the whole run was searched and the pricing note is not cited: %+v", answer.Citations)
	}
	if cited[f.pricingDeal] {
		t.Fatalf("a deal was cited as evidence about activities: %+v", answer.Citations)
	}
	if answer.Prevalence == nil || answer.Prevalence.Matched != 5 || answer.Prevalence.Of != 13 {
		t.Errorf("prevalence = %+v, want 5 of 13", answer.Prevalence)
	}
}

// A record the reader lost access to after the run was saved is not served,
// and the answer says part of the set was out of reach instead of stating a
// share of what was left.
func TestReportEvidenceSaysWhenPartOfTheRunIsOutOfReach(t *testing.T) {
	e := SetupSearch(t)
	f := seedEvidenceFixture(t, e)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	hidden := f.pricingCalls[0]
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants', captured_by = $2 WHERE id = $1`,
		hidden, "human:"+e.Rep3.String()); err != nil {
		t.Fatalf("limiting the call's audience: %v", err)
	}

	answer := mustSearchEvidence(ctx, t, e,
		`{"run_id":"`+runID.String()+`","cell":["call"],"query":"pricing"}`)

	if citedIDs(answer)[hidden] {
		t.Fatalf("a call the reader may no longer read was cited: %+v", answer.Citations)
	}
	if !evidenceNoted(answer, agents.CodeRecordsOutOfReach) {
		t.Errorf("notes = %+v, want the unreached part named", answer.Notes)
	}
	if answer.Coverage != agents.CoveragePartialDegraded || answer.Prevalence != nil {
		t.Errorf("coverage %q, prevalence %+v: a share of a partly readable set was stated",
			answer.Coverage, answer.Prevalence)
	}
	if !evidenceNoted(answer, agents.CodePrevalenceRefused) {
		t.Errorf("notes = %+v, want the refused prevalence said", answer.Notes)
	}
}

// A record with no text to judge is an abstention, and it too keeps the
// share from being stated.
func TestReportEvidenceReportsAbstentions(t *testing.T) {
	e := SetupSearch(t)
	seedEvidenceFixture(t, e)
	silent := e.SeedID(t, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
		VALUES ($1, 'call', now() - interval '1 hour', 'manual', 'human:x')`)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	answer := mustSearchEvidence(ctx, t, e,
		`{"run_id":"`+runID.String()+`","cell":["call"],"query":"pricing"}`)

	if len(answer.Abstentions) != 1 || answer.Abstentions[0].ID != silent {
		t.Fatalf("abstentions = %+v, want only the call with no text", answer.Abstentions)
	}
	if answer.Tally.Unjudged != 1 || !evidenceNoted(answer, agents.CodeRecordsUnjudged) {
		t.Errorf("tally %+v, notes %+v: the abstention is not reported", answer.Tally, answer.Notes)
	}
	if answer.Prevalence != nil {
		t.Errorf("prevalence %+v stated over a set with a record nobody judged", answer.Prevalence)
	}
}

// A run id nothing was saved under is not found, the same answer the report
// drawer gives.
func TestReportEvidenceForAnUnknownRunIsNotFound(t *testing.T) {
	e := SetupSearch(t)
	_, err := searchEvidence(e.evidenceReader(), e,
		`{"run_id":"`+ids.NewV7().String()+`","query":"pricing"}`)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an unknown run answered %v, want not found", err)
	}
}

// The bound holds on both retrieval lanes. The meaning lane ranks the closest
// vectors whatever their words, so a record outside the bound embedded with
// the query's own text would lead an unbounded page; bounded, it never
// appears, and neither does its word-for-word lexical twin.
func TestABoundedSearchFindsNothingOutsideItsRecords(t *testing.T) {
	e := SetupSearch(t)
	embedder := fakeEmbedder(t, ai.NewFakeClient())
	inside := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Solar Panels', 'manual', 'human:x')`)
	outside := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Solar Grid', 'manual', 'human:x')`)
	for id, text := range map[ids.UUID]string{inside: "rooftop install", outside: "solar grid"} {
		if _, err := e.Store.UpsertEmbedding(e.Admin(), "contact", id, text, embedder); err != nil {
			t.Fatal(err)
		}
	}
	retriever := search.NewRetriever(e.Store, embedder)

	unbounded, err := retriever.Search(e.Admin(), retrieval.Query{Text: "solar grid", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(unbounded.Hits) == 0 || unbounded.Hits[0].Ref.ID != outside || !unbounded.SemanticRanking {
		t.Fatalf("the unbounded control should lead with the outside record by meaning: %+v", unbounded)
	}

	bounded, err := retriever.Search(e.Admin(), retrieval.Query{
		Text: "solar grid", Limit: 10, Within: []ids.UUID{inside},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range bounded.Hits {
		if hit.Ref.ID != inside {
			t.Fatalf("a bounded search returned %s from outside its records: %+v", hit.Ref.ID, bounded.Hits)
		}
	}
	if len(bounded.Hits) != 1 {
		t.Errorf("the one record inside the bound matches and was not found: %+v", bounded.Hits)
	}

	empty, err := retriever.Search(e.Admin(), retrieval.Query{Text: "solar grid", Limit: 10, Within: []ids.UUID{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Hits) != 0 {
		t.Errorf("a search bounded to no records found %+v", empty.Hits)
	}
}
