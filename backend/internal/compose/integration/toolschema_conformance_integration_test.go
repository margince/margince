// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The load-bearing half of the exact output schemas.
//
// A declared schema nothing checks is a comment, and this surface now declares
// thirty of them. The unit tests prove the DERIVATION is right — that a struct
// tag becomes the wire name a caller reads — and prove the CHECKER is right, on
// documents written by hand. Neither of them can prove the thing a client
// actually depends on: that the bytes a handler produces, against a real
// database, satisfy the schema its tool advertised.
//
// So this suite invokes tools for real and holds each answer to its own schema,
// through the same ResultDefect the dispatcher uses — a second spelling here
// would be a second definition of "conforms", and the one that mattered would be
// the server's.
//
// An empty answer is worth checking and is much of what a fresh workspace
// yields. `{"records":[]}` and `{"records":null}` are one Go value apart and
// only one of them keeps the schema, which is exactly the class of defect a
// hand-built result map used to hide.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WHY THIS RUNS AS A HUMAN, and what that buys. The confirm-first tier gates
// AGENT principals: a 🟡 call from a passport is staged and answers with an
// approval reference rather than a result, so an agent-driven sweep could only
// ever reach the 🟢 half of the surface. A human principal is admitted by RBAC
// and receives the result itself — which is the document these schemas describe,
// and the same document an agent gets once a human has released the call. So the
// sweep covers the confirm-first tools too.
//
// What it still cannot reach is named rather than implied: the tools whose
// handler needs a seam this lane does not stand up — a live calendar
// (book_meeting), an outbound mail or channel provider (send_email,
// send_message), a crawl runner or model path (enrich), a drafting brain
// (draft_follow_ups_for). Their declared schemas are checked by derivation and
// by the encoder-agreement test in the agents package, but not against a live
// handler.
func TestToolAnswersReachableWithoutApprovalSatisfyTheirSchemas(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	// Every record these reads are about is created through the tool surface,
	// which holds create_record's own answer to its schema on the way — a
	// write's read-back is a result like any other, and it is the one every
	// write tool shares.
	person := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"person","fields":{"full_name":"Schema Conformance"}}`)
	org := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"organization","fields":{"display_name":"Conformance GmbH"}}`)
	lead := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"lead","fields":{"email":"lead@conformance.example"}}`)
	deal := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"deal","fields":{"name":"Conformance renewal","pipeline_id":"`+
			pipeline.String()+`","stage_id":"`+open.String()+`"}}`)
	activity := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"activity","fields":{"kind":"note","body":"to be relinked"}}`)
	// The vocabulary half: coined here so update_tag below has a word to rename.
	tag := coinTagThroughTheToolSurface(ctx, t, registry, `{"name":"conformance","color":"teal"}`)
	// Records the confirm-first sweep consumes, each its own so one tool's write
	// cannot decide whether the next tool's call is even legal.
	promotable := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"lead","fields":{"email":"promote@conformance.example","full_name":"Promo Table"}}`)
	spare := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"person","fields":{"full_name":"To Be Archived"}}`)
	duplicate := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"person","fields":{"full_name":"Schema Conformance (dup)"}}`)
	project := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"project","fields":{"name":"Conformance project","organization_id":"`+
			org.String()+`"}}`)

	// The two producers the forecast tools read. The lane's seat already holds
	// what the real admin role holds over a forecast — create + read, plus read
	// on source health — so nothing about the grants stops these calls; what
	// stopped them is that four of the five read a row nobody had written.
	openingSnapshot, closingSnapshot := twoFrozenForecasts(ctx, t, e)
	oneFinishedInputCheck(ctx, t, e)

	// A second word for update_tag and a third for merge_tags, so one
	// vocabulary write cannot decide what the next tool reads: a merge RETIRES
	// its source, and the tools below still have to find `tag` where they left
	// it.
	renamable := coinTagThroughTheToolSurface(ctx, t, registry, `{"name":"to-be-renamed"}`)
	foldable := coinTagThroughTheToolSurface(ctx, t, registry, `{"name":"to-be-folded"}`)

	// The brief is a persisted read-model, so the lane assembles one for this
	// rep before reading it: read_brief never ranks, and an unassembled brief
	// answers not-found rather than an empty queue.
	briefItem, briefEvidence := annotatable(t, snapshotBriefRun(ctx, t, e))

	// One waiting proposal for the queue tools, staged through the engine that
	// stages every other one. The verdict below is a REJECTION: it exercises the
	// same answer shape and releases nothing, so the sweep does not perform a
	// deal write as a side effect of checking a schema.
	waiting := stageOneProposal(ctx, t, e, deal)

	calls := []struct{ tool, args string }{
		{"list_pipelines", `{}`},
		{"read_brief", `{}`},
		// The night writing back onto the morning it just read. The narrative
		// is the run-level half and the item names one the snapshot above
		// ranked, so the call exercises both arms rather than the empty one.
		{"annotate_brief", `{"narrative":"A quiet night.","items":[{"item_id":"` +
			briefItem.String() + `","finding":"Nothing moved on this deal.","cited_evidence":["` +
			briefEvidence.String() + `"]}]}`},
		// Who the caller is, and who else they can hand work to. Both answer
		// about the acting human rather than about a record, so the empty
		// object is the whole input; list_colleagues also runs narrowed,
		// because the filtered answer is the one that can come back truncated.
		{"whoami", `{}`},
		{"list_colleagues", `{}`},
		{"list_colleagues", `{"q":"nothing here matches this"}`},

		// An enumeration, narrowed and unnarrowed. The narrowed one is the
		// answer worth holding to the shape: a filter that reached no SQL still
		// returns a well-formed page, of the wrong rows.
		{"list_records", `{"record_type":"deal","filters":{"pipeline_id":"` + pipeline.String() + `"}}`},
		{"list_records", `{"record_type":"person","limit":5}`},
		{"run_report", `{"report":"deals-by-stage"}`},
		// The typed engine's own door, over the same pipeline the fixture above
		// already holds a deal in — so the count answers 1 rather than an
		// unproven empty page.
		{"run_analytics_query", `{"entity":"pipeline-current","measures":[{"fn":"count","as":"deals"}]}`},
		// No arguments: the directory answers what the installation composed, so
		// there is nothing for a caller to narrow by — and the empty object is
		// the shape the schema declares.
		{"list_channel_providers", `{}`},
		// No arguments, and there can be none: the schema declares an empty
		// object and forbids every other member, so the second call its
		// neighbours carry is impossible rather than merely redundant. It is
		// also the one tool here that reads nothing off its context, so the
		// seal supplies freshness, evidence and warnings unaided — an absent
		// one would show here first.
		{"check_location_support", `{}`},
		{"search_records", `{"q":"Conformance"}`},
		{"search_records", `{"q":"Conformance","record_type":"person","limit":5}`},
		// A query that matches nothing: the empty answer has to keep the shape
		// too, and it is the one a caller is most likely to mis-read.
		{"search_records", `{"q":"nothing here matches this"}`},
		{"read_record", `{"record_type":"person","id":"` + person.String() + `"}`},
		// A plan that matches rows and one that matches none. The empty answer
		// is the one worth holding to the shape: `coverage` is the field a
		// caller reads before believing a short result, and it is not omitempty
		// precisely so an absent one cannot read as a complete one.
		{"query_workspace", `{"plan":{"version":"v1","target":"deal","where":[{"field":"status","op":"eq","value":"open"}]}}`},
		{"query_workspace", `{"plan":{"version":"v1","target":"deal","where":[{"field":"name","op":"eq","value":"nothing here matches this"}]}}`},
		// The vocabulary the two plans above are written against. It takes no
		// arguments and needs no seam the lane is missing, so it is a call here
		// rather than a waiver: the census does not accept "registered and
		// unproven" for a tool this sweep can simply invoke.
		{"describe_query_vocabulary", `{}`},
		// The vocabulary run_report's own plan is written against, for the same
		// reason: no arguments, no seam this lane is missing, so the census gets
		// a call rather than a waiver.
		{"describe_report_vocabulary", `{}`},
		{"read_record", `{"record_type":"deal","id":"` + deal.String() + `"}`},
		// A ranked sweep that finds rows and one that finds none. The empty page
		// is the one worth pinning: it still carries `coverage` and `notes`, and
		// `notes` is not omitempty precisely so `null` cannot read as "nothing to
		// report" on a page that was in fact degraded.
		{"search_context", `{"query":"Conformance","record_types":["person"]}`},
		{"search_context", `{"query":"nothing here matches this"}`},
		// A payload that resolves and one that resolves to nothing. The second is
		// the answer a caller acts on by CREATING a record, so its shape is the
		// one a mis-read costs the most.
		{"resolve_entities", `{"candidates":[{"kind":"person","ref":"a","name":"Conformance"}]}`},
		{"resolve_entities", `{"candidates":[{"kind":"organization","emails":["nobody@nowhere.example"]}]}`},
		{"catch_me_up_on", `{"record_type":"deal","record_id":"` + deal.String() + `"}`},
		{"prep_for_meeting", `{"record_type":"deal","record_id":"` + deal.String() + `"}`},
		// An ACTIVITY anchor takes the other road through the walk — the event
		// is dereferenced to the records it names and the prep is built around
		// one of them — so it reaches sections a record anchor never emits, and
		// its answer still has to keep the shape this server advertises.
		{"prep_for_meeting", `{"record_type":"activity","record_id":"` + activity.String() + `"}`},
		{"whats_slipping_this_week", `{}`},
		// The open-promise review, unnarrowed and narrowed to one owner. The
		// narrowed one is the answer worth holding: `as_of` and each item's
		// `state` are what a reader judges lateness by, and a narrowing that
		// reached no SQL still returns a well-formed set — of everyone's
		// promises.
		{"review_commitments", `{}`},
		{"review_commitments", `{"limit":5}`},
		// The delivery briefing. The project seeded above has no owner and no
		// target end date, so this is the answer WITH gaps in it — the shape a
		// caller acts on, and the one where a missing `gaps` member would read
		// as work cleared for handover.
		{"prepare_handoff", `{"project_id":"` + project.String() + `"}`},
		// The project page read as a tool. The fixture project has no deal, no
		// seat and no task, so this is the answer with every list EMPTY — the
		// shape where a null list would read as "unknown".
		{"read_project_360", `{"project_id":"` + project.String() + `"}`},
		{"at_risk_relationships", `{}`},
		{"who_knows", `{"person_id":"` + person.String() + `"}`},
		{"account_coverage", `{"deal_id":"` + deal.String() + `"}`},
		{"intro_path_to", `{"organization_id":"` + org.String() + `"}`},
		{"qualify_lead", `{"record_id":"` + lead.String() + `"}`},
		// The passthrough shapes, whose declared schema is a GUARANTEED SUBSET
		// rather than a type this module marshals. They are the ones a unit test
		// cannot check at all: nothing here builds the document, so the only way
		// to know the subset is true is to ask the real handler.
		{"check_availability", `{"from":"2026-01-05T09:00:00Z","to":"2026-01-05T17:00:00Z"}`},
		{"relink_activity", `{"activity_id":"` + activity.String() + `","entity_type":"person","entity_id":"` +
			person.String() + `"}`},
		// The batch forms answer a count-and-ids shape of their own. The thread
		// one names a key no activity carries, which is a well-formed empty
		// answer; the set one names the activity above, onto an organization.
		{"relink_thread", `{"thread_key":"thread:conformance","entity_type":"person","entity_id":"` +
			person.String() + `"}`},
		{"relink_activities", `{"activity_ids":["` + activity.String() + `"],"entity_type":"organization","entity_id":"` +
			org.String() + `"}`},
		{"disqualify_lead", `{"lead_id":"` + lead.String() + `"}`},
		{"log_activity", `{"kind":"note","body":"conformance","links":[{"entity_type":"deal","entity_id":"` +
			deal.String() + `"}]}`},
		{"create_task", `{"subject":"conformance","links":[{"entity_type":"deal","entity_id":"` +
			deal.String() + `"}]}`},
		{"update_record", `{"record_type":"person","id":"` + person.String() +
			`","fields":{"title":"Head of Conformance"}}`},
		{"progress_deal", `{"deal_id":"` + deal.String() + `","to_stage_id":"` + open.String() +
			`","note":"still open"}`},
		{"advance_deal", `{"deal_id":"` + deal.String() + `","to_stage_id":"` + open.String() + `"}`},
		// The confirm-first tools, reachable here because this runs as a human.
		{"promote_lead", `{"lead_id":"` + promotable.String() + `","trigger":"human_qualify"}`},
		{"archive_record", `{"record_type":"person","id":"` + spare.String() + `"}`},
		{"merge_records", `{"record_type":"person","source_id":"` + duplicate.String() +
			`","target_id":"` + person.String() + `"}`},
		{"advance_project_phase", `{"project_id":"` + project.String() + `","to_phase":"pursuing"}`},
		// Renaming the word coined above. Both tag-vocabulary writes are reachable
		// here: AdminPerms carries tag create/read/update/delete.
		{"update_tag", `{"tag_id":"` + renamable.String() + `","name":"conformance-renamed","color":"amber"}`},
		// One record's place in the vocabulary: enumerated, read one at a time,
		// applied, read back off the record, and taken off again. Every one of
		// these was waived as needing `tag.read` that "this lane's seat does not
		// carry" — it carries it, and coining the tag above under that same seat
		// is the proof. A waiver whose reason has expired reads as covered while
		// certifying nothing.
		{"list_tags", `{}`},
		{"get_tag", `{"tag_id":"` + tag.String() + `"}`},
		{"apply_tag", `{"tag_id":"` + tag.String() +
			`","record_type":"person","record_id":"` + person.String() + `"}`},
		{"get_record_tags", `{"record_type":"person","record_id":"` + person.String() + `"}`},
		{"remove_tag", `{"tag_id":"` + tag.String() +
			`","record_type":"person","record_id":"` + person.String() + `"}`},
		// The one vocabulary verb that reaches a human first, invoked as one:
		// this lane runs as a human, so it returns the merge itself rather than
		// an approval reference — which is the document the schema describes.
		//
		// Worth the third tag it costs. `TagMergeResult` is two bare counts and
		// is the shape of nothing else on this surface — `update_tag` answers a
		// `Tag` — so a waiver here leaves the one answer where `0` and absent
		// are the same JSON unproven against a live handler.
		{"merge_tags", `{"tag_id":"` + foldable.String() +
			`","into_tag_id":"` + tag.String() + `"}`},
		// The forecast: what a period is expected to close, the difference
		// between two frozen states of it, what the inputs were checked
		// against, what they still need, and how current the sources are. Each
		// carries a shape a schema alone cannot hold — two slices that are
		// empty and never null, and a source that was not read carrying no date
		// where a checked one carries one.
		{"forecast_readings", `{}`},
		{"forecast_movement", `{"from":"` + openingSnapshot.String() +
			`","to":"` + closingSnapshot.String() + `","reading":"open"}`},
		{"forecast_input_checks", `{}`},
		{"list_input_checks", `{}`},
		{"data_coverage", `{}`},
		// The report surface: the block grammar a document is written in, and
		// a document composed through it. Invoked with a text-only block on
		// purpose — the figure-bearing kinds resolve against a run, and what
		// this lane holds is the ENVELOPE the composer answers in, which is
		// the same for a document whose blocks carry figures and one whose do
		// not. Both tools take the read scope and neither stages anything.
		{"describe_report_blocks", `{}`},
		{"compose_analytics_report", `{"blocks":[{"kind":"title","text":"Pipeline this quarter"}]}`},
		// The confirm-first queue read back through its own doors.
		{"list_approvals", `{}`},
		{"read_approval", `{"staged_action_id":"` + waiting.String() + `"}`},
		{"decide_approval", `{"staged_action_id":"` + waiting.String() + `","decision":"reject"}`},
	}
	assertEveryRegisteredToolIsAccountedFor(t, registry, calls)

	for _, call := range calls {
		t.Run(call.tool+" "+call.args, func(t *testing.T) {
			spec, registered := registry.Spec(call.tool)
			if !registered {
				t.Fatalf("%s is not registered, so this call proves nothing", call.tool)
			}
			out, err := registry.Invoke(ctx, call.tool, json.RawMessage(call.args))
			if err != nil {
				t.Fatalf("%s(%s): %v", call.tool, call.args, err)
			}
			if defect := agents.ResultDefect(spec.OutputSchema, out); defect != "" {
				t.Errorf("%s answered %s, which does not keep the schema this server advertises for it: %s",
					call.tool, out, defect)
			}
			// AC-MCP-7: the envelope is asserted against the REAL answer of a
			// real call, not against its declaration. A declared envelope
			// nothing checks is the comment this whole change replaces.
			assertEnvelopePopulated(t, call.tool, out)
		})
	}
}

// unreachableInThisLane names the tools this sweep CANNOT invoke, and why. Each
// needs a seam the lane does not stand up — a live calendar, an outbound mail or
// channel provider, a crawl runner, a drafting brain — so a call would exercise
// a stub rather than a handler.
//
// It is a waiver rather than prose because the census below reads it: a tool
// listed here and then made reachable fails as loudly as one that was never
// covered, so the list cannot quietly outlive its reason.
var unreachableInThisLane = gatekit.Waive(map[string]string{
	"preview_import": "needs an object store to put the source file in; this lane composes none, " +
		"so the call would exercise the refusal rather than the handler",
	"read_import_run": "needs a seat holding import_run.read, which this lane's seat does not " +
		"carry — a migration run is an admin-scoped object, and granting it here would widen the " +
		"authority every other tool in the sweep runs under",
	"read_import_report":   "needs a run that has been dry-run, which needs the object store above",
	"commit_import":        "confirm-first, and needs the object store above to reach a committable run",
	"book_meeting":         "needs a live calendar provider",
	"send_email":           "needs an outbound mail provider",
	"send_account_email":   "needs an outbound mail provider, and a send-capable mailbox for its pre-flight",
	"send_message":         "needs an outbound channel provider",
	"draft_email":          "needs a drafting model path",
	"draft_follow_ups_for": "needs a drafting model path",
	"enrich":               "needs a crawl runner and a model path",
	// A bundle is minted by a producer that stages several proposals as one act
	// (a site read), never by a caller — so there is no bundle_id to answer here
	// without standing that producer up.
	"decide_approval_bundle": "needs a multi-proposal producer to mint a bundle",
})

// twoFrozenForecasts freezes two states of one period, so the movement tool has
// a difference to classify.
//
// Taken through the store's own writer rather than by INSERT: a snapshot is a
// headline plus the rows it was summed from, and the movement tool reads BOTH
// sides back — so a hand-built row would prove the classifier against a shape
// production never writes.
//
// `recheck` rather than `daily`, which the arbiter admits once per period,
// scope and local day: two same-day daily snapshots are the collision that
// index exists to cause, and a recheck is deliberately additional.
func twoFrozenForecasts(ctx context.Context, t *testing.T, e *Env) (ids.UUID, ids.UUID) {
	t.Helper()
	store := forecasting.NewStore(compose.InstallationDB(e.Pool))
	period, err := forecasting.ResolvePeriod(
		forecasting.PeriodQuarter, time.Now().UTC(), 1, time.UTC)
	if err != nil {
		t.Fatalf("resolving the period the snapshots describe: %v", err)
	}
	freeze := func(open int64, at time.Time) ids.UUID {
		var id ids.UUID
		if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			id, err = store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
				Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
				Trigger: forecasting.TriggerRecheck, BaseCurrency: "EUR",
				Readings: forecasting.Readings{OpenMinor: open},
				TakenAt:  at,
			})
			return err
		}); err != nil {
			t.Fatalf("freezing a forecast for the movement tool: %v", err)
		}
		return id
	}
	taken := time.Now().UTC()
	return freeze(100_000, taken.Add(-time.Hour)), freeze(140_000, taken)
}

// oneFinishedInputCheck records a completed nightly pass, so the tools that
// report one have a run to read.
//
// Through the store's own three writers, in the order the checker uses them: a
// run is OPENED, says how far it reached into each source, and is then closed
// with a verdict. A row inserted directly would answer the read while proving
// nothing about the pass that produces it.
func oneFinishedInputCheck(ctx context.Context, t *testing.T, e *Env) {
	t.Helper()
	store := assurance.NewStore(compose.InstallationDB(e.Pool))
	asOf := time.Now().UTC()
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		run, err := store.StartRun(ctx, tx, asOf)
		if err != nil {
			return err
		}
		// Both arms of the coverage answer: a source the pass read through,
		// which carries a date, and one it could not, which must not. That
		// second arm is what makes `checks_incomplete` a different answer from
		// a clean pipeline.
		if err := store.RecordCoverage(ctx, tx, run, assurance.SourceCoverage{
			Source: "mail", State: assurance.CoverageChecked, CheckedThrough: &asOf,
		}); err != nil {
			return err
		}
		if err := store.RecordCoverage(ctx, tx, run, assurance.SourceCoverage{
			Source: "offers", State: assurance.CoverageUnavailable,
		}); err != nil {
			return err
		}
		return store.FinishRun(ctx, tx, run, 1, 0, 0, assurance.StatusComplete, "ready")
	}); err != nil {
		t.Fatalf("recording a finished input check for the forecast tools: %v", err)
	}
}

// stageOneProposal puts a single proposal in the queue through the approvals
// engine itself — the writer every producer uses — rather than an INSERT, so
// what the queue tools read back is a row production would have written.
func stageOneProposal(ctx context.Context, t *testing.T, e *Env, deal ids.UUID) ids.ApprovalID {
	t.Helper()
	staged, err := approvals.NewService(compose.InstallationDB(e.Pool)).Stage(ctx, approvals.StageInput{
		Kind:           "close_date_correction",
		ProposedChange: json.RawMessage(`{"expected_close_date":"2026-12-31"}`),
		DiffHash:       ids.NewV7().String(),
		TargetType:     "deal",
		TargetID:       deal,
		Summary:        "Move the expected close date to 2026-12-31",
	})
	if err != nil {
		t.Fatalf("staging a proposal for the queue tools: %v", err)
	}
	return staged
}

// assertEveryRegisteredToolIsAccountedFor is what makes AC-MCP-7's "every
// registered tool" true rather than aspirational.
//
// The sweep's own table cannot say it: a tool added tomorrow is simply absent
// from it, and an absent tool is indistinguishable from a passing one. So the
// census is derived from the REGISTRY — every spec is either invoked below or
// named as unreachable with its reason, and nothing may be both.
func assertEveryRegisteredToolIsAccountedFor(t *testing.T, registry *agents.Registry, calls []struct{ tool, args string }) {
	t.Helper()
	// create_record is exercised by the fixture setup rather than by the table:
	// every record the calls below read was made through it, and
	// createThroughTheToolSurface holds each of those answers to its schema and
	// its envelope on the way past.
	invoked := map[string]bool{"create_record": true, "create_tag": true}
	for _, call := range calls {
		invoked[call.tool] = true
	}
	for _, spec := range registry.Specs() {
		excused := unreachableInThisLane.Waived(t, spec.Name)
		switch {
		case invoked[spec.Name] && excused:
			t.Errorf("%s is both invoked here and excused as unreachable — one of the two is stale", spec.Name)
		case !invoked[spec.Name] && !excused:
			t.Errorf("%s is registered but never invoked here, so nothing holds its result to the schema "+
				"or the envelope. Add a call above, or name the seam it needs in unreachableInThisLane.", spec.Name)
		}
	}
	// And the other direction: an entry no registered tool reached is a reason
	// describing a tool that is gone, which reads as covered while certifying
	// nothing.
	unreachableInThisLane.AssertAllMatched(t)
}

// createThroughTheToolSurface makes one record and returns its id, holding the
// write's own answer to its declared schema before reading the id out of it.
func createThroughTheToolSurface(ctx context.Context, t *testing.T, registry *agents.Registry, args string) ids.UUID {
	t.Helper()
	spec, registered := registry.Spec("create_record")
	if !registered {
		t.Fatal("create_record is not registered")
	}
	out, err := registry.Invoke(ctx, "create_record", json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_record(%s): %v", args, err)
	}
	if defect := agents.ResultDefect(spec.OutputSchema, out); defect != "" {
		t.Fatalf("create_record answered %s, which does not keep its own schema: %s", out, defect)
	}
	assertEnvelopePopulated(t, "create_record", out)
	var created struct {
		Data struct {
			ID ids.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unreadable create_record answer %s: %v", out, err)
	}
	return created.Data.ID
}

// coinTagThroughTheToolSurface creates one tag through create_tag and returns
// its id, holding that tool's own answer to its schema on the way.
//
// Mirrors createThroughTheToolSurface for the same reason: update_tag needs a
// tag that exists, and the call list is assembled before any call runs, so the
// id has to come from somewhere. Coining it through the tool rather than seeding
// SQL means create_tag is exercised too — which is why the census pre-seeds it
// as invoked, exactly as it does for create_record.
func coinTagThroughTheToolSurface(ctx context.Context, t *testing.T, registry *agents.Registry, args string) ids.UUID {
	t.Helper()
	spec, registered := registry.Spec("create_tag")
	if !registered {
		t.Fatal("create_tag is not registered")
	}
	out, err := registry.Invoke(ctx, "create_tag", json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_tag(%s): %v", args, err)
	}
	if defect := agents.ResultDefect(spec.OutputSchema, out); defect != "" {
		t.Fatalf("create_tag answered %s, which does not keep its own schema: %s", out, defect)
	}
	assertEnvelopePopulated(t, "create_tag", out)
	var created struct {
		Data struct {
			TagID ids.UUID `json:"tag_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unreadable create_tag answer %s: %v", out, err)
	}
	return created.Data.TagID
}

// A conformance suite that could not fail is the thing it exists to prevent, so
// it is shown failing: a schema deliberately declaring a member no result
// carries has to be reported, against a REAL answer rather than a fixture.
func TestTheConformanceCheckFailsAgainstAMisdeclaredSchema(t *testing.T) {
	e := Setup(t)
	DealFixture(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	out, err := registry.Invoke(ctx, "list_pipelines", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_pipelines: %v", err)
	}
	for name, misdeclared := range map[string]string{
		"a required member no result carries":           `{"type":"object","required":["invented"]}`,
		"an envelope member declared as the wrong type": `{"type":"object","properties":{"trust":{"type":"integer"}}}`,
		"a payload member declared as the wrong type": `{"type":"object","properties":{"data":{"type":"object",` +
			`"properties":{"pipelines":{"type":"string"}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if defect := agents.ResultDefect(json.RawMessage(misdeclared), out); defect == "" {
				t.Error("the misdeclaration was reported as satisfied — this suite cannot see " +
					"the defect it exists to catch")
			}
		})
	}
}

// annotatable names one item of the assembled brief for annotate_brief to
// write onto, with one of the evidence ids that item already carries.
//
// Both halves come from the run rather than being minted here, because the tool
// refuses a finding that cites evidence the item does not hold — that refusal is
// the feature, so a fixture inventing an id would prove the sweep can dodge it.
//
// It fails rather than skipping when the run ranked nothing or ranked an item
// with no evidence: either way the call would exercise a refusal, and this sweep
// exists to certify the answer shape a real call produces.
func annotatable(t *testing.T, run briefs.BriefRun) (item, evidence ids.UUID) {
	t.Helper()
	if len(run.Items) == 0 {
		t.Fatal("the assembled brief ranked nothing, so annotate_brief has no item to write onto — " +
			"seed a deal this rep owns that the ranker will pick up")
	}
	first := run.Items[0]
	if len(first.EvidenceIDs) == 0 {
		t.Fatal("the top brief item cites no evidence, so annotate_brief would refuse the finding — " +
			"the ranker's evidence-or-omit rule should make this unreachable")
	}
	return first.ID, first.EvidenceIDs[0]
}

// snapshotBriefRun assembles one morning brief for the acting rep, the way the
// human home surface does.
//
// read_brief re-reads a persisted run and never ranks — that is the contract's
// own rule, not a limitation of this lane — so without a run the sweep would be
// certifying a not-found instead of an answer.
func snapshotBriefRun(ctx context.Context, t *testing.T, e *Env) briefs.BriefRun {
	t.Helper()
	engine := briefs.NewBriefEngine(e.Pool, people.NewStore(e.DB()))
	// The reader's own instant, because brief_run is keyed by LOCAL DAY and
	// read_brief asks for today's: briefseam.go reads `LatestRun(ctx,
	// time.Now().UTC())`, which resolves a calendar day and matches
	// `WHERE user_id = $1 AND local_day = $2`. A run stamped with any other day
	// is a run that read cannot see, so a fixed instant here does not freeze
	// what the lane certifies — it dates the fixture, and the sweep certifies a
	// not-found from the morning after.
	//
	// Sharing the clock is what keeps the two ends honest; nothing here rests on
	// WHICH day it is, only that one run exists under the day the read will ask
	// for. The two agree by asking the same question, not by holding one answer
	// between them — so a reader that moves to an injected clock leaves this
	// behind, silently and the same way. Follow it here when it does.
	run, err := engine.SnapshotRun(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("assembling a brief run for the acting rep: %v", err)
	}
	return run
}
