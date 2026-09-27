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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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

func searchEvidence(ctx context.Context, e *SearchEnv, args string) (agents.SearchReportEvidenceResult, sealedResult, error) {
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	out, err := registry.Invoke(ctx, "search_report_evidence", json.RawMessage(args))
	if err != nil {
		return agents.SearchReportEvidenceResult{}, sealedResult{}, err
	}
	spec, _ := registry.Spec("search_report_evidence")
	if defect := agents.ResultDefect(spec.OutputSchema, out); defect != "" {
		return agents.SearchReportEvidenceResult{}, sealedResult{}, fmt.Errorf("the answer breaks its own schema: %s", defect)
	}
	var sealed sealedResult
	if err := json.Unmarshal(out, &sealed); err != nil {
		return agents.SearchReportEvidenceResult{}, sealedResult{}, fmt.Errorf("the result is not an envelope: %w (%s)", err, out)
	}
	var answer agents.SearchReportEvidenceResult
	if err := json.Unmarshal(sealed.Data, &answer); err != nil {
		return agents.SearchReportEvidenceResult{}, sealedResult{}, fmt.Errorf("unreadable payload %s: %w", sealed.Data, err)
	}
	return answer, sealed, nil
}

func mustSearchEvidence(ctx context.Context, t *testing.T, e *SearchEnv, args string) agents.SearchReportEvidenceResult {
	t.Helper()
	answer, _, err := searchEvidence(ctx, e, args)
	if err != nil {
		t.Fatalf("search_report_evidence %s\n  → %v", args, err)
	}
	return answer
}

// hideFromTheReader limits an activity's audience to a colleague, so the
// reader may no longer read it.
func hideFromTheReader(t *testing.T, e *SearchEnv, id ids.UUID) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants', captured_by = $2 WHERE id = $1`,
		id, "human:"+e.Rep3.String()); err != nil {
		t.Fatalf("limiting the activity's audience: %v", err)
	}
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

// Without a cell the search covers every cell the run served, so the note's
// mention of pricing becomes evidence; the cell above narrowed it away.
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
// and every figure is over what they can still read — which the answer says
// on every call, not only on this one.
func TestReportEvidenceCountsOnlyWhatTheReaderCanRead(t *testing.T) {
	e := SetupSearch(t)
	f := seedEvidenceFixture(t, e)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)
	hidden := f.pricingCalls[0]
	hideFromTheReader(t, e, hidden)

	answer := mustSearchEvidence(ctx, t, e,
		`{"run_id":"`+runID.String()+`","cell":["call"],"query":"pricing"}`)

	if citedIDs(answer)[hidden] {
		t.Fatalf("a call the reader may no longer read was cited: %+v", answer.Citations)
	}
	if !evidenceNoted(answer, agents.CodeOverReadableRecords) {
		t.Errorf("notes = %+v, want the readable-records basis said", answer.Notes)
	}
	if answer.Prevalence == nil || answer.Prevalence.Matched != 3 || answer.Prevalence.Of != 6 {
		t.Errorf("prevalence = %+v, want 3 of the 6 calls the reader can read", answer.Prevalence)
	}
}

// A record the reader cannot read changes nothing in the answer, however well
// it matches: its presence must not be learnable from any field.
func TestAHiddenRecordChangesNothingInTheAnswer(t *testing.T) {
	e := SetupSearch(t)
	seedEvidenceFixture(t, e)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)
	args := `{"run_id":"` + runID.String() + `","cell":["call"],"query":"pricing"}`
	_, before, err := searchEvidence(ctx, e, args)
	if err != nil {
		t.Fatal(err)
	}

	hidden := e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'call', 'Weekly check-in', 'Pricing was the only topic', now() - interval '1 hour', 'manual', 'human:x')`)
	hideFromTheReader(t, e, hidden)
	_, after, err := searchEvidence(ctx, e, args)
	if err != nil {
		t.Fatal(err)
	}
	if string(before.Data) != string(after.Data) {
		t.Errorf("a record hidden from the reader changed the answer:\nbefore %s\nafter  %s", before.Data, after.Data)
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

// An abstention serves an id, and a served id is a read: the envelope sources
// it, which is what the passport's read bound charges.
func TestAnAbstentionIsChargedAsARead(t *testing.T) {
	e := SetupSearch(t)
	seedEvidenceFixture(t, e)
	silent := e.SeedID(t, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
		VALUES ($1, 'call', now() - interval '1 hour', 'manual', 'human:x')`)
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	_, sealed, err := searchEvidence(ctx, e, `{"run_id":"`+runID.String()+`","cell":["call"],"query":"pricing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !sealedNames(sealed, silent) {
		t.Errorf("the abstention %s was served and not charged: evidence %+v", silent, sealed.Evidence)
	}
}

// The provider the tool reads back through marks an activity the reader may
// know of but not read as content_state "withheld" — the value the read-back
// drops a record on.
func TestTheProviderMarksAnUnreadableActivityWithheld(t *testing.T) {
	e := SetupSearch(t)
	f := seedEvidenceFixture(t, e)
	hideFromTheReader(t, e, f.pricingCalls[0])
	record, err := compose.NewProvider(e.Pool).Read(e.evidenceReader(),
		datasource.EntityRef{Type: datasource.EntityActivity, ID: f.pricingCalls[0]})
	if err != nil {
		t.Fatalf("reading the activity back: %v", err)
	}
	var fields struct {
		ContentState string  `json:"content_state"`
		Body         *string `json:"body"`
	}
	if err := json.Unmarshal(record.Fields, &fields); err != nil {
		t.Fatal(err)
	}
	if fields.ContentState != "withheld" || fields.Body != nil {
		t.Errorf("content_state %q, body %v: want the content withheld", fields.ContentState, fields.Body)
	}
}

// Without a cell, a cell the privacy floor withholds stays closed: its
// records are never evidence, and the answer says cells were left out.
func TestTheWholeRunNeverOpensAWithheldCell(t *testing.T) {
	e := SetupSearch(t)
	seedEvidenceFixture(t, e)
	var small []ids.UUID
	for range 2 {
		small = append(small, e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'Site visit', 'Pricing came up on site', now() - interval '1 hour', 'manual', 'human:x')`))
	}
	ctx := e.evidenceReader()
	runID := saveActivitiesByKind(ctx, t, e)

	answer := mustSearchEvidence(ctx, t, e, `{"run_id":"`+runID.String()+`","query":"pricing"}`)

	cited := citedIDs(answer)
	for _, id := range small {
		if cited[id] {
			t.Fatalf("a record of a withheld cell was cited: %s", id)
		}
	}
	if !evidenceNoted(answer, agents.CodeCellsWithheld) || answer.Prevalence != nil {
		t.Errorf("notes %+v, prevalence %+v: the withheld cells are not said", answer.Notes, answer.Prevalence)
	}
}

// Searching a whole run keeps the grouping's own narrowing. A rep who may
// measure only themselves asks projects-by-phase by owner and is answered
// about their own projects; the whole run must not reach a colleague's.
func TestTheWholeRunKeepsTheNarrowingItsGroupingBrought(t *testing.T) {
	e := SetupSearch(t)
	company := e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Sunworks', 'manual', 'human:x')`)
	project := func(owner ids.UUID, name string) ids.UUID {
		return e.SeedID(t, `INSERT INTO project (id, owner_id, name, company_id, source, captured_by)
			VALUES ($1, $2, $3, $4, 'manual', 'human:x')`, owner, name, company)
	}
	var own, colleagues []ids.UUID
	for i := range 6 {
		own = append(own, project(e.Rep1, fmt.Sprintf("Solar rollout %d", i)))
		colleagues = append(colleagues, project(e.Rep3, fmt.Sprintf("Solar retrofit %d", i)))
	}
	grants := searchReadGrants()
	for _, object := range []string{"project", "forecast"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeOwn},
	})
	q := analyticsquery.Query{
		Entity: "projects-by-phase", GroupBy: []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "projects"}},
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

	answer := mustSearchEvidence(ctx, t, e, `{"run_id":"`+runID.String()+`","query":"solar"}`)

	cited := citedIDs(answer)
	for _, id := range colleagues {
		if cited[id] {
			t.Fatalf("the whole run reached a colleague's project %s the rep may not measure", id)
		}
	}
	if answer.Tally.Matched != len(own) {
		t.Errorf("tally %+v, want the rep's own %d projects matched", answer.Tally, len(own))
	}
}

// A run id nothing was saved under is not found, the same answer the report
// drawer gives.
func TestReportEvidenceForAnUnknownRunIsNotFound(t *testing.T) {
	e := SetupSearch(t)
	_, _, err := searchEvidence(e.evidenceReader(), e,
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
