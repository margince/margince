// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The lead first-response SLA (formulas §18): the clock, the three first-
// response triggers, the derived state, and the at-most-once breach scan.
// The scan's SKIP LOCKED contract and the sla_state filter's SQL are only
// real against Postgres.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLeadCreatedAt seeds a working lead whose clock started at createdAt.
func (e *promoteConsentEnv) seedLeadCreatedAt(t *testing.T, email string, createdAt time.Time) ids.LeadID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, email, status, source, captured_by, owner_id, created_at)
		 VALUES ($1, 'Lena Lead', lower($2), 'new', 'inbound', 'human:x', $3, $4)`,
		id, email, e.user, createdAt); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.LeadKind](id)
}

// enableFirstResponseSLA switches the opt-in target on through the real
// settings write, with the default target.
func (e *promoteConsentEnv) enableFirstResponseSLA(t *testing.T) {
	t.Helper()
	on := true
	if _, err := e.store.UpdateLeadSettings(e.ctx, UpdateLeadSettingsInput{FirstResponseEnabled: &on}); err != nil {
		t.Fatalf("enable first-response SLA: %v", err)
	}
}

func (e *promoteConsentEnv) countEvents(t *testing.T, eventType string, entity ids.UUID) int {
	t.Helper()
	var n int
	if err := e.store.tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM event_outbox
			  WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2`, eventType, entity.String()).Scan(&n)
	}); err != nil {
		t.Fatalf("count %s events: %v", eventType, err)
	}
	return n
}

// The scan marks a breached lead exactly once: a second pass finds nothing,
// and a lead answered before its deadline is never a breach.
func TestSLAScanEscalatesEachBreachOnce(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC()
	overdue := e.seedLeadCreatedAt(t, "overdue@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	fresh := e.seedLeadCreatedAt(t, "fresh@example.test", now.Add(-time.Hour))
	answered := e.seedLeadCreatedAt(t, "answered@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	if _, err := e.store.RecordLeadFirstResponse(e.ctx, answered, now.Add(-DefaultFirstResponseTarget-30*time.Minute)); err != nil {
		t.Fatalf("record first response: %v", err)
	}

	breaches, err := e.store.ScanLeadSLA(e.ctx, now)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(breaches) != 1 || breaches[0].LeadID != overdue {
		t.Fatalf("breaches = %+v, want exactly the overdue lead %s", breaches, overdue)
	}
	if breaches[0].OwnerID == nil || *breaches[0].OwnerID != ids.From[ids.UserKind](e.user) {
		t.Errorf("breach owner = %v, want the lead's owner — the escalation target", breaches[0].OwnerID)
	}
	if n := e.countEvents(t, "lead.sla_breached", overdue.UUID); n != 1 {
		t.Errorf("lead.sla_breached events = %d, want 1", n)
	}

	again, err := e.store.ScanLeadSLA(e.ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second scan re-escalated %+v; a breach is at most once per occurrence", again)
	}
	if n := e.countEvents(t, "lead.sla_breached", fresh.UUID); n != 0 {
		t.Errorf("a lead inside its target was escalated")
	}
}

// The derived state and the list filter agree: an overdue lead reads
// breached on its own row AND is what sla_state=breached lists.
func TestSLAStateReadsAndFiltersAlike(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC()
	overdue := e.seedLeadCreatedAt(t, "overdue@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	e.seedLeadCreatedAt(t, "fresh@example.test", now.Add(-time.Hour))

	lead, err := e.store.GetLead(e.ctx, overdue, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if lead.SlaState == nil || *lead.SlaState != crmcontracts.LeadSlaStateBreached || lead.SlaDeadlineAt == nil {
		t.Errorf("overdue lead sla_state=%v deadline=%v, want breached with a deadline", lead.SlaState, lead.SlaDeadlineAt)
	}
	breached := crmcontracts.ListLeadsParamsSlaState(crmcontracts.LeadSlaStateBreached)
	page, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{SLAState: &breached})
	if err != nil {
		t.Fatalf("list breached: %v", err)
	}
	if len(page) != 1 || page[0].Id != lead.Id {
		t.Errorf("sla_state=breached lists %d leads, want only %s", len(page), overdue)
	}
	within := crmcontracts.ListLeadsParamsSlaState(crmcontracts.LeadSlaStateWithinTarget)
	page, _, err = e.store.ListLeads(e.ctx, ListLeadsInput{SLAState: &within})
	if err != nil {
		t.Fatalf("list within: %v", err)
	}
	if len(page) != 1 || page[0].Id == lead.Id {
		t.Errorf("sla_state=within_target lists %d leads incl. the overdue one", len(page))
	}
}

func TestDefaultLeadQueueOrdersSLAThenScoreDeterministically(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC().Truncate(time.Second)
	previousClock := leadSLAClock
	leadSLAClock = func() time.Time { return now }
	t.Cleanup(func() { leadSLAClock = previousClock })

	breachedLow := e.seedLeadCreatedAt(t, "breached-low@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	breachedHigh := e.seedLeadCreatedAt(t, "breached-high@example.test", now.Add(-DefaultFirstResponseTarget-2*time.Hour))
	atRisk := e.seedLeadCreatedAt(t, "at-risk@example.test", now.Add(-DefaultFirstResponseTarget+30*time.Minute))
	within := e.seedLeadCreatedAt(t, "within@example.test", now.Add(-time.Hour))
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET score = CASE id WHEN $1 THEN 10 WHEN $2 THEN 80 WHEN $3 THEN 100 ELSE 100 END
		  WHERE id IN ($1, $2, $3, $4)`, breachedLow, breachedHigh, atRisk, within); err != nil {
		t.Fatalf("setting queue scores: %v", err)
	}

	limit := 2
	rows, page, err := e.store.ListLeads(e.ctx, ListLeadsInput{Limit: &limit})
	if err != nil {
		t.Fatalf("list default queue: %v", err)
	}
	if !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first queue page = %+v, want a continuation cursor", page)
	}
	cursor := page.NextCursor
	// The queue is one snapshot across pages. Without the as-of time in the
	// cursor, atRisk crosses into breached here, moves ahead of the cursor and
	// disappears from the result set.
	leadSLAClock = func() time.Time { return now.Add(2 * time.Hour) }
	next, lastPage, err := e.store.ListLeads(e.ctx, ListLeadsInput{Limit: &limit, Cursor: &cursor})
	if err != nil {
		t.Fatalf("list second queue page: %v", err)
	}
	if lastPage.HasMore {
		t.Fatalf("second queue page = %+v, want the final page", lastPage)
	}
	rows = append(rows, next...)
	want := []ids.LeadID{breachedHigh, breachedLow, atRisk, within}
	if len(rows) != len(want) {
		t.Fatalf("queue rows = %d, want %d", len(rows), len(want))
	}
	for i, id := range want {
		if ids.UUID(rows[i].Id) != id.UUID {
			t.Errorf("queue[%d] = %s, want %s", i, rows[i].Id, id)
		}
	}
}

func TestLeadQuickFindIncludesExactEmailAndLinkedIn(t *testing.T) {
	e := setupPromoteConsent(t)
	now := time.Now().UTC()
	emailLead := e.seedLeadCreatedAt(t, "find-me@example.test", now)
	linkedInLead := e.seedLeadCreatedAt(t, "other@example.test", now)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET linkedin_url = 'https://www.linkedin.com/in/find-me'
		  WHERE id = $1`, linkedInLead); err != nil {
		t.Fatalf("setting LinkedIn URL: %v", err)
	}

	for query, want := range map[string]ids.LeadID{
		"find-me@example.test":                 emailLead,
		"https://www.linkedin.com/in/find-me/": linkedInLead,
	} {
		rows, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Query: &query})
		if err != nil {
			t.Fatalf("quick-find %q: %v", query, err)
		}
		if len(rows) != 1 || ids.UUID(rows[0].Id) != want.UUID {
			t.Errorf("quick-find %q = %+v, want %s", query, rows, want)
		}
	}
}

// A human moving the lead off `new` is a first response; the stamp is set
// once and a later status change does not move it. Disqualifying an
// unanswered lead is an explicit disposition and stamps it too.
func TestFirstResponseFromHumanStatusChangeAndDisposition(t *testing.T) {
	e := setupPromoteConsent(t)
	now := time.Now().UTC()
	worked := e.seedLeadCreatedAt(t, "worked@example.test", now.Add(-time.Hour))
	status := "contacted"
	after, err := e.store.UpdateLead(e.ctx, worked, UpdateLeadInput{Status: &status})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if after.FirstResponseAt == nil {
		t.Fatal("a human status change off new must stamp first_response_at")
	}
	if after.SlaState != nil {
		t.Errorf("an answered lead reads sla_state=%v, want null — it owes nothing", *after.SlaState)
	}
	stamped := *after.FirstResponseAt
	back := "new"
	if _, err := e.store.UpdateLead(e.ctx, worked, UpdateLeadInput{Status: &back}); err != nil {
		t.Fatalf("update back: %v", err)
	}
	if _, err := e.store.UpdateLead(e.ctx, worked, UpdateLeadInput{Status: &status}); err != nil {
		t.Fatalf("update again: %v", err)
	}
	again, err := e.store.GetLead(e.ctx, worked, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if again.FirstResponseAt == nil || !again.FirstResponseAt.Equal(stamped) {
		t.Errorf("first_response_at moved from %v to %v; the first stamp is the one that counts", stamped, again.FirstResponseAt)
	}

	dropped := e.seedLeadCreatedAt(t, "dropped@example.test", now.Add(-time.Hour))
	closed, err := e.store.DisqualifyLead(e.ctx, dropped, DisqualifyLeadInput{})
	if err != nil {
		t.Fatalf("disqualify: %v", err)
	}
	if closed.FirstResponseAt == nil {
		t.Error("disqualifying is an explicit disposition and must stamp first_response_at")
	}
}

// The bus is unordered: a reply that happened at 09:00 may be processed
// after one from 10:00. The FIRST response is the earliest, so the later
// delivery moves the stamp back, and a later reply never moves it forward.
func TestFirstResponseKeepsTheEarliestReplyWhateverTheDeliveryOrder(t *testing.T) {
	e := setupPromoteConsent(t)
	// Truncated to what timestamptz actually stores. Postgres keeps
	// microseconds and Go carries nanoseconds, so an untruncated stamp comes
	// back a few hundred nanoseconds short and Equal fails — on roughly 999
	// runs in 1000, whenever time.Now() does not happen to land on a
	// microsecond boundary. The test is about which reply WINS, not about
	// clock resolution.
	now := time.Now().UTC().Truncate(time.Microsecond)
	lead := e.seedLeadCreatedAt(t, "ordered@example.test", now.Add(-3*time.Hour))
	late := now.Add(-time.Hour)
	early := now.Add(-2 * time.Hour)
	if set, err := e.store.RecordLeadFirstResponse(e.ctx, lead, late); err != nil || !set {
		t.Fatalf("first delivery: set=%t err=%v", set, err)
	}
	if set, err := e.store.RecordLeadFirstResponse(e.ctx, lead, early); err != nil || !set {
		t.Fatalf("earlier reply delivered second: set=%t err=%v — it must win", set, err)
	}
	if set, err := e.store.RecordLeadFirstResponse(e.ctx, lead, now); err != nil || set {
		t.Fatalf("a later reply: set=%t err=%v — it must not move the stamp", set, err)
	}
	got, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstResponseAt == nil || !got.FirstResponseAt.Equal(early) {
		t.Errorf("first_response_at = %v, want the earliest reply %v", got.FirstResponseAt, early)
	}
}

// seedLeadActivity links one activity to a lead, exactly as a capture would
// have committed it — and WITHOUT running the subscriber that projects
// lead.first_response_at from it. That gap is the whole subject: the scan has
// to survive the window between the activity's commit and the projection.
func (e *promoteConsentEnv) seedLeadTouch(t *testing.T, lead ids.LeadID, kind, direction, capturedBy, source string, at time.Time) {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	// direction is NULL for a note: the column's CHECK admits inbound and
	// outbound and nothing else, and a note has neither.
	var carried *string
	if direction != "" {
		carried = &direction
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by)
		 VALUES ($1, $2, 'Reply', $3, $4, $5, $6)`,
		id, kind, carried, at, source, capturedBy); err != nil {
		t.Fatalf("seeding the %s activity: %v", kind, err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`,
		id, lead); err != nil {
		t.Fatalf("linking the activity to the lead: %v", err)
	}
}

// A lead answered before its deadline is not breached, even when the scan runs
// before the projection catches up.
//
// lead.first_response_at is written by a subscriber reacting to
// activity.captured, so it lags the activity's own commit by the bus's
// latency. An outbound reply committed at 11:59 against a 12:00 deadline can
// still be unprojected when a 12:01 scan runs — and the scan owns the
// consequence: a breach event, an escalation task, and a number on somebody's
// review. Every activity below is committed WITHOUT running the subscriber,
// which is the window this reproduces.
func TestTheScanReadsTheActivitiesRatherThanTheProjection(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	started := time.Now().UTC().Add(-DefaultFirstResponseTarget - time.Hour)

	for name, seed := range map[string]func(lead ids.LeadID, deadline time.Time){
		// A person's outbound reply, one minute inside the deadline: the case
		// from the report.
		"a human's outbound reply": func(lead ids.LeadID, deadline time.Time) {
			e.seedLeadTouch(t, lead, "email", "outbound", "human:"+e.user.String(), "manual", deadline.Add(-time.Minute))
		},
		// A note the rep typed into the composer counts too (§18.1) — the
		// stepper shows the lead as contacted, so it must not go on to breach
		// as if nobody had answered.
		"a note a rep logged": func(lead ids.LeadID, deadline time.Time) {
			e.seedLeadTouch(t, lead, "note", "", "human:"+e.user.String(), "manual", deadline.Add(-time.Minute))
		},
	} {
		t.Run(name, func(t *testing.T) {
			lead := e.seedLeadCreatedAt(t, name+"@sla.test", started)
			state, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatalf("read the lead's deadline: %v", err)
			}
			if state.SlaDeadlineAt == nil {
				t.Fatal("the lead has no first-response deadline, so this case tests nothing")
			}
			seed(lead, *state.SlaDeadlineAt)

			breaches, err := e.store.ScanLeadSLA(e.ctx, time.Now().UTC())
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			for _, b := range breaches {
				if b.LeadID == lead {
					t.Fatal("a lead answered before its deadline was marked breached — the scan read " +
						"lead.first_response_at, which the subscriber had not projected yet")
				}
			}
			// And the scan stamped the column itself from what it found, so the
			// subscriber's later write is the replay it already answers as a
			// no-op rather than a second first response.
			after, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if after.FirstResponseAt == nil {
				t.Error("the scan found the response and did not record it, so the next scan re-reads " +
					"every activity on this lead to reach the same answer")
			}
			if after.SlaState != nil && *after.SlaState == crmcontracts.LeadSlaStateBreached {
				t.Error("the lead reads sla_state=breached")
			}
		})
	}
}

// And the scan still breaches a lead nobody answered: a version that returned
// "answered" for everything would pass the test above while switching the
// whole target off.
func TestTheScanStillBreachesALeadNobodyAnswered(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	lead := e.seedLeadCreatedAt(t, "unanswered@sla.test", time.Now().UTC().Add(-DefaultFirstResponseTarget-time.Hour))

	// An activity that is NOT a first response: an inbound the lead sent, and
	// an agent's outbound with nothing to respond to (§18.1's anti-pollution
	// case). Neither may keep the breach away.
	state, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil || state.SlaDeadlineAt == nil {
		t.Fatalf("read the lead's deadline: %v (%v)", err, state.SlaDeadlineAt)
	}
	e.seedLeadTouch(t, lead, "email", "outbound", "agent:scheduler", "connector", state.SlaDeadlineAt.Add(-time.Minute))

	breaches, err := e.store.ScanLeadSLA(e.ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	found := false
	for _, b := range breaches {
		if b.LeadID == lead {
			found = true
		}
	}
	if !found {
		t.Fatal("a lead nobody answered was not breached — an agent's cold outbound is not a first " +
			"response, and treating it as one turns the whole target off")
	}
}

// A reply AFTER the deadline is not an answer in time, and must not rescue the
// lead from a breach it has already earned.
func TestALateReplyDoesNotRescueTheLead(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	lead := e.seedLeadCreatedAt(t, "late@sla.test", time.Now().UTC().Add(-DefaultFirstResponseTarget-time.Hour))
	state, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil || state.SlaDeadlineAt == nil {
		t.Fatalf("read the lead's deadline: %v (%v)", err, state.SlaDeadlineAt)
	}
	e.seedLeadTouch(t, lead, "email", "outbound", "human:"+e.user.String(), "manual", state.SlaDeadlineAt.Add(time.Minute))

	breaches, err := e.store.ScanLeadSLA(e.ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, b := range breaches {
		if b.LeadID == lead {
			return
		}
	}
	t.Fatal("a lead answered a minute LATE was not breached — the re-check must be bounded by the " +
		"deadline, or the target only ever fires for a lead nobody answered at all")
}
