// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal page and the contact page, asked about the SAME quiet deal, over the
// real stores.
//
// They disagreed in front of a reader: the contact page said "No next step with
// them on an open deal — book a meeting", the deal card said to go complete a
// forecast-assurance task the product had minted for itself. Both were reading
// real rows. The contact page was right, and the deal card was outranked by
// housekeeping.
//
// Two things are proved here that no unit test can reach, because both are SQL:
// that a system-minted task is excluded from the card's open-work read, and
// that the moment on the contact page survives a page full of such reminders.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestBothPagesAgreeTheNextStepIsAMeeting(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)

	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Annabelle Malherbe", "source": "ui",
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact = %d %v", status, contact)
	}
	contactID, _ := contact["id"].(string)
	// The seat is what lets the card name somebody. Without the role it is a
	// stakeholder with no place in the opening order, and the card correctly
	// falls back to naming nobody — which is the behaviour the fixture used to
	// get by accident.
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "deal_stakeholder", "deal_id": dealID, "contact_id": contactID,
		"role": "champion", "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("stake the champion on the deal = %d", status)
	}

	// Contact 23 days ago and nothing since: the deal is quiet, and the last
	// thing that happened was a meeting that has already been held.
	quiet := time.Now().UTC().AddDate(0, 0, -23).Format(time.RFC3339)
	var meeting AnyMap
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Detailabstimmung Rollout-Plan",
		"occurred_at": quiet, "source": "manual",
		"links": []AnyMap{
			{"entity_type": "deal", "entity_id": dealID},
			{"entity_type": "contact", "entity_id": contactID},
		},
	}, nil, &meeting); status != http.StatusCreated {
		t.Fatalf("log the last meeting = %d %v", status, meeting)
	}

	// The housekeeping row, written the way the product writes one: the system
	// principal, and the remediation origin the recency clocks already exclude.
	// It is REAL open work and it stays on the task list — what it must not do
	// is answer "has anybody agreed a next step".
	mintSystemTask(t, e, dealID, contactID)

	card := readStatus(t, e, dealID)
	next, _ := card["next"].(map[string]any)
	if next["action"] != "create_task" {
		t.Fatalf("next = %v, want the meeting filed as work rather than the housekeeping task", next)
	}
	args, _ := next["arguments"].(map[string]any)
	if args["subject"] != "Book a meeting with Annabelle Malherbe" {
		t.Fatalf("the card does not name the champion to meet: %v", args)
	}
	if reason, _ := next["reason"].(string); reason == "" ||
		!strings.Contains(reason, "nothing is booked") || !strings.Contains(reason, "champion") {
		t.Fatalf("the reason does not say why now or why them: %q", next["reason"])
	}
	// The pulse's own fact: nothing inbound is flagged, which is what the
	// sentence above it is allowed to claim and all it is allowed to claim.
	if card["reply_to"] != nil {
		t.Fatalf("reply_to = %v on a deal with no unanswered inbound mail", card["reply_to"])
	}

	// The same question, asked of the contact page. It reached this answer
	// before the deal card did; the point is that they now say it together.
	moment := readMoment(t, e, contactID)
	if moment["rule"] != "missing_next_step" {
		t.Fatalf("the contact page opens on %v, want the same finding the deal card just made", moment["rule"])
	}

	// Performing the move settles BOTH surfaces, because the task it files is
	// linked to the deal and to the contact.
	if status := e.Call(t, "POST", "/v1/tasks", args, nil, nil); status != http.StatusCreated {
		t.Fatalf("create the meeting task from the card = %d", status)
	}
	afterNext, _ := readStatus(t, e, dealID)["next"].(map[string]any)
	if afterNext["action"] != "open_task" {
		t.Fatalf("after filing the meeting the card offers %v, want the work it just created", afterNext)
	}
	if rule := readMoment(t, e, contactID)["rule"]; rule == "missing_next_step" {
		t.Fatal("the contact page still reports no next step after one was agreed with them")
	}
}

func TestAPageOfRemindersDoesNotBuryTheNextStep(t *testing.T) {
	// The under-recognition case: a contact with more system reminders than one
	// page holds, and one human promise among them. A rung that filtered the
	// page it was handed would report a missing next step, because the promise
	// it is looking for sits on page two under plain urgency ordering.
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)

	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Annabelle Malherbe", "source": "ui",
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact = %d %v", status, contact)
	}
	contactID, _ := contact["id"].(string)
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "deal_stakeholder", "deal_id": dealID, "contact_id": contactID,
		"role": "champion", "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("stake the champion on the deal = %d", status)
	}

	// The human promise, dated furthest out, so urgency alone would sort it
	// last of all.
	far := time.Now().UTC().AddDate(0, 0, 90).Format(time.RFC3339)
	if status := e.Call(t, "POST", "/v1/tasks", AnyMap{
		"subject": "Send the revised rollout plan", "due_at": far, "source": "ui",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contactID}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("file the human promise = %d", status)
	}
	// Thirty reminders, every one due sooner than the promise. The page holds
	// twenty-five.
	for i := range 30 {
		mintSystemReminder(t, e, contactID, i)
	}

	if rule := readMoment(t, e, contactID)["rule"]; rule == "missing_next_step" {
		t.Fatal("a page of reminders hid the one promise a colleague actually made, " +
			"and the page reported a next step that exists as missing")
	}
}

// mintSystemTask writes the forecast-assurance shape: the product filing work
// about a deal, under the system principal, with the remediation origin.
func mintSystemTask(t *testing.T, e *apptest.AppEnv, dealID, contactID string) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		WITH t AS (
		  INSERT INTO activity (id, kind, subject, occurred_at, is_done, source, captured_by, origin, audience)
		  VALUES (uuidv7(), 'task', '1 forecast input needs attention', now(), false,
		          'system', 'system:forecast-assurance', 'system_remediation', 'workspace')
		  RETURNING id
		)
		INSERT INTO activity_link (activity_id, entity_type, deal_id, contact_id)
		SELECT t.id, l.entity_type, l.deal_id, l.contact_id FROM t,
		  (VALUES ('deal', $1::uuid, NULL::uuid), ('contact', NULL::uuid, $2::uuid)) AS l(entity_type, deal_id, contact_id)`,
		dealID, contactID); err != nil {
		t.Fatalf("mint the system task: %v", err)
	}
}

// mintSystemReminder writes one check-in reminder against a contact, due soon,
// so a page of them competes with a human promise for the section's first page.
func mintSystemReminder(t *testing.T, e *apptest.AppEnv, contactID string, n int) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		WITH t AS (
		  INSERT INTO activity (id, kind, subject, occurred_at, due_at, is_done, source, captured_by, origin, audience)
		  VALUES (uuidv7(), 'task', $1, now(), now() + make_interval(mins => $2::int), false,
		          'system', 'system:time-scan', 'system_remediation', 'workspace')
		  RETURNING id
		)
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		SELECT t.id, 'contact', $3::uuid FROM t`,
		fmt.Sprintf("Check in — no activity since last month (%d)", n), n+1, contactID); err != nil {
		t.Fatalf("mint a system reminder: %v", err)
	}
}

// readMoment is the contact page's opening card, which is the surface this
// change had to agree with.
func readMoment(t *testing.T, e *apptest.AppEnv, contactID string) AnyMap {
	t.Helper()
	var page AnyMap
	if status := e.Call(t, "GET", "/v1/contacts/"+contactID+"/360", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("contact 360 = %d %v", status, page)
	}
	moment, _ := page["moment"].(map[string]any)
	if moment == nil {
		t.Fatalf("the contact page opens on no moment at all: %v", page)
	}
	return moment
}
