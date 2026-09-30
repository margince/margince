// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A quiet-account reminder closes when the activity that proves the silence
// over is captured. The reminder is minted by the real time scan, the touch is
// written by the real activity writer, and the activity.captured envelope that
// writer put on the outbox is handed to the real workflow engine.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logTouch writes one genuine email through the activity store, linked to the
// given records, and returns its id.
func logTouch(t *testing.T, e *Env, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	subject := "Re: renewal"
	direction := "outbound"
	touch, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &workedRecently,
		Links: links, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the touch: %v", err)
	}
	return ids.UUID(touch.Id)
}

// deliverCaptured hands the activity.captured envelope the writer put on the
// outbox for this activity to the workflow engine, the way the relay does.
func deliverCaptured(t *testing.T, e *Env, activity ids.UUID) {
	t.Helper()
	var raw []byte
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT envelope FROM event_outbox
		  WHERE envelope->>'type' = 'activity.captured' AND envelope->'entity'->>'id' = $1`,
		activity.String()).Scan(&raw); err != nil {
		t.Fatalf("reading the activity.captured envelope: %v", err)
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the envelope: %v", err)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("delivering activity.captured: %v", err)
	}
}

// openTaskCountOn counts the tasks on one record's timeline still open.
func openTaskCountOn(t *testing.T, e *Env, entityType string, entity ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `
		SELECT count(*) FROM activity a
		JOIN activity_link al ON al.activity_id = a.id
		WHERE al.entity_type = $1
		  AND coalesce(al.contact_id, al.company_id, al.deal_id, al.lead_id) = $2
		  AND a.kind = 'task' AND a.is_done = false AND a.archived_at IS NULL`, entityType, entity)
}

func TestMailToAnAccountsContactClosesTheAccountsQuietReminder(t *testing.T) {
	e := Setup(t)
	conn := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Reminded Account", nil)
	deal := e.SeedDeal(t, "Reminded Account Renewal", pipeline, open, nil)
	attachDealToCompany(t, conn, deal, company)
	contact := e.SeedContact(t, "Buyer At The Account", nil)
	seedEmployment(t, conn, contact, company)
	backdateCreatedAt(t, conn, "company", company, longEstablished)
	backdateCreatedAt(t, conn, "deal", deal, longEstablished)
	linkQuietTouch(t, conn, e.WS, "company", company)
	seedNoActivityReminder(t, conn, e.WS)
	runEligibilityScan(t, e)
	if got := openTaskCountOn(t, e, "company", company); got != 1 {
		t.Fatalf("open reminders on the account after the scan = %d, want 1", got)
	}

	due := workedRecently.AddDate(0, 0, 7)
	subject := "Send the revised offer"
	human, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	})
	if err != nil {
		t.Fatalf("logging the rep's own task: %v", err)
	}

	touch := logTouch(t, e, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact})
	deliverCaptured(t, e, touch)

	if got := openTaskCountOn(t, e, "company", company); got != 1 {
		t.Fatalf("open tasks on the account = %d, want 1 — the reminder closes, the rep's own task stays", got)
	}
	var humanDone bool
	if err := conn.QueryRow(context.Background(),
		`SELECT is_done FROM activity WHERE id = $1`, ids.UUID(human.Id)).Scan(&humanDone); err != nil {
		t.Fatal(err)
	}
	if humanDone {
		t.Fatal("the rep's own task was completed — only the system's reminders are the system's to close")
	}

	deliverCaptured(t, e, touch)
	if got := openTaskCountOn(t, e, "company", company); got != 1 {
		t.Fatalf("open tasks after a redelivery = %d, want still 1", got)
	}
}

// A touch closes the reminders on the records it reaches and no others: mail
// to a deal's stakeholder answers the stakeholder's reminder, not the deal's.
func TestATouchClosesOnlyTheRemindersOnTheRecordsItReaches(t *testing.T) {
	e := Setup(t)
	conn := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Unaccounted Deal", pipeline, open, nil)
	contact := e.SeedContact(t, "Stakeholder", nil)
	seedStakeholderSeat(t, conn, contact, deal)
	backdateCreatedAt(t, conn, "deal", deal, longEstablished)
	backdateCreatedAt(t, conn, "contact", contact, longEstablished)
	linkQuietTouch(t, conn, e.WS, "deal", deal)
	linkQuietTouch(t, conn, e.WS, "contact", contact)
	seedNoActivityReminder(t, conn, e.WS)
	runEligibilityScan(t, e)
	if openTaskCountOn(t, e, "deal", deal) != 1 || openTaskCountOn(t, e, "contact", contact) != 1 {
		t.Fatal("the scan did not remind about both the deal and its stakeholder")
	}
	// A system task that is not a quiet reminder: it asks for something the
	// mail does not answer.
	request := ids.NewV7()
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, source_system, source_id)
		 VALUES ($1, 'task', 'Send the contract they asked for', $2, 'system', 'system:email-request',
		         'email_request', $3)`, request, workedRecently, request.String()); err != nil {
		t.Fatalf("seeding the request task: %v", err)
	}
	linkTouch(t, conn, e.WS, request, "contact", contact)

	deliverCaptured(t, e, logTouch(t, e, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact}))
	if got := openTaskCountOn(t, e, "contact", contact); got != 1 {
		t.Errorf("open tasks on the stakeholder = %d, want 1 — mail to them answers the reminder, not the request task", got)
	}
	if got := openTaskCountOn(t, e, "deal", deal); got != 1 {
		t.Errorf("open reminders on the deal = %d, want 1 — the mail was not filed on the deal", got)
	}

	deliverCaptured(t, e, logTouch(t, e, activities.ActivityLinkInput{EntityType: "deal", EntityID: deal}))
	if got := openTaskCountOn(t, e, "deal", deal); got != 0 {
		t.Errorf("open reminders on the deal after mail filed on it = %d, want 0", got)
	}
}

// Only a genuine touch ends a silence. A note the product wrote itself would
// not move the scan's anchor, so it does not close the reminder either.
func TestTheEnginesOwnRowDoesNotCloseAQuietReminder(t *testing.T) {
	e := Setup(t)
	conn := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Engine Noted Deal", pipeline, open, nil)
	backdateCreatedAt(t, conn, "deal", deal, longEstablished)
	linkQuietTouch(t, conn, e.WS, "deal", deal)
	seedNoActivityReminder(t, conn, e.WS)
	runEligibilityScan(t, e)

	engineNote := ids.NewV7()
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'note', 'Stage changed', $2, 'system', 'system:time-scan')`,
		engineNote, workedRecently); err != nil {
		t.Fatalf("seeding the engine's note: %v", err)
	}
	linkTouch(t, conn, e.WS, engineNote, "deal", deal)
	payload, err := json.Marshal(map[string]string{"kind": "note"})
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: "activity.captured", OccurredAt: workedRecently,
		Entity: kevents.EntityRef{Type: "activity", ID: engineNote}, Payload: payload,
	}); err != nil {
		t.Fatalf("delivering the engine note's activity.captured: %v", err)
	}

	if got := openTaskCountOn(t, e, "deal", deal); got != 1 {
		t.Fatalf("open reminders on the deal = %d, want 1 — the engine's own note is not engagement", got)
	}
}

// logTouchAt is logTouch at a chosen instant: a history import delivers mail
// long after it was sent, dated when it was sent.
func logTouchAt(t *testing.T, e *Env, at time.Time, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	subject := "Imported thread"
	direction := "outbound"
	touch, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at,
		Links: links, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the touch: %v", err)
	}
	return ids.UUID(touch.Id)
}

// seedRemindedQuietDeal is a quiet deal whose reminder the scan has already
// minted, anchored on quietSince.
func seedRemindedQuietDeal(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	conn := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Imported History Deal", pipeline, open, nil)
	backdateCreatedAt(t, conn, "deal", deal, longEstablished)
	linkQuietTouch(t, conn, e.WS, "deal", deal)
	seedNoActivityReminder(t, conn, e.WS)
	runEligibilityScan(t, e)
	if got := openTaskCountOn(t, e, "deal", deal); got != 1 {
		t.Fatalf("open reminders after the scan = %d, want 1", got)
	}
	return deal
}

// Mail dated at or before the reminder's anchor does not end the silence the
// reminder is about, and the anchor has not moved, so closing it would leave
// the silence reported by nobody.
func TestMailDatedBeforeTheAnchorLeavesTheQuietReminderOpen(t *testing.T) {
	for name, at := range map[string]time.Time{
		"older":         quietSince.AddDate(0, 0, -3),
		"at the anchor": quietSince,
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			deal := seedRemindedQuietDeal(t, e)

			deliverCaptured(t, e, logTouchAt(t, e, at, activities.ActivityLinkInput{EntityType: "deal", EntityID: deal}))
			if got := openTaskCountOn(t, e, "deal", deal); got != 1 {
				t.Fatalf("open reminders = %d, want 1 — mail from before the silence began does not answer it", got)
			}

			runEligibilityScan(t, e)
			if got := taskCountOn(t, e, "deal", deal); got != 1 {
				t.Fatalf("reminder tasks after a rescan = %d, want still 1", got)
			}
		})
	}
}

// Mail sent after the anchor but captured only after the reminder was minted —
// a history import finishing late — proves the reminder wrong and closes it.
// The deal is still quiet since that mail, so the next scan asks once more,
// naming the later date.
func TestDelayedMailNewerThanTheAnchorClosesTheQuietReminder(t *testing.T) {
	e := Setup(t)
	deal := seedRemindedQuietDeal(t, e)
	sent := quietSince.AddDate(0, 0, 5)

	deliverCaptured(t, e, logTouchAt(t, e, sent, activities.ActivityLinkInput{EntityType: "deal", EntityID: deal}))
	if got := openTaskCountOn(t, e, "deal", deal); got != 0 {
		t.Fatalf("open reminders = %d, want 0 — the mail answers the reminder", got)
	}

	runEligibilityScan(t, e)
	if got := openTaskCountOn(t, e, "deal", deal); got != 1 {
		t.Fatalf("open reminders after a rescan = %d, want 1 — the deal is quiet again since %s", got, sent.Format(time.DateOnly))
	}
	runEligibilityScan(t, e)
	if got := taskCountOn(t, e, "deal", deal); got != 2 {
		t.Fatalf("reminder tasks after a second rescan = %d, want 2 — the closed one and one for the new silence", got)
	}
}
