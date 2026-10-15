// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Our mail on a customer's thread answers them only when it is sent to them.
// Mail to a second correspondent on the same thread, or a Bcc copy, leaves the
// customer unanswered; a reply that copies them answers.
func TestAReplyOnTheThreadToSomebodyElseDoesNotAnswerTheSender(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	thread, sender := "thread-"+ids.NewV7().String(), "dana-"+e.rep.String()+"@client.test"
	inbound := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Question', now() - interval '3 hours', $2, $3, 'seed', 'system')`,
		inbound, thread, sender)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), inbound, contact)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Re: Question', now() - interval '2 hours', $2, 'eli@partner.test', 'seed', 'system')`,
		ids.NewV7(), thread)

	elsewhere, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if elsewhere.Answered != before.Answered {
		t.Fatalf("the answered count moved %d → %d: our mail to another correspondent on the thread answered the sender",
			before.Answered, elsewhere.Answered)
	}

	blind := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Re: Question', now() - interval '90 minutes', $2, $3, 'seed', 'system')`,
		blind, thread, sender)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'bcc', $2)`, blind, sender)
	bcc, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if bcc.Answered != before.Answered {
		t.Fatalf("the answered count moved %d → %d: a Bcc copy answered the sender", before.Answered, bcc.Answered)
	}

	copied := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Re: Question', now() - interval '1 hour', $2, 'eli@partner.test', 'seed', 'system')`,
		copied, thread)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', $2)`, copied, sender)

	answered, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if answered.Answered != before.Answered+1 {
		t.Fatalf("the answered count is %d, want %d: a reply that copies the sender answers them",
			answered.Answered, before.Answered+1)
	}
}
