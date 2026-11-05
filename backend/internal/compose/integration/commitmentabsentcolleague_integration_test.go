// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The model's promiser is checked against the promised sentence and the seat
// roster before a task is filed. A reply that names the sender for work the
// sentence gives a colleague not on the mail files no task for the sender.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// robinSeat gives the second rep the name the promised sentences use. They are
// never on the mail.
func robinSeat(t *testing.T, e *Env) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(t.Context(), `UPDATE app_user SET display_name = 'Robin Weber' WHERE id = $1`, e.Rep2); err != nil {
		t.Fatal(err)
	}
}

func TestAReplyNamingTheSenderForAnAbsentColleaguesWorkFilesNoTask(t *testing.T) {
	for name, sentence := range map[string]string{
		"first name":                    "Product demo on the 5th at 10:00, online with Robin.",
		"full name":                     "Robin Weber runs the product demo on the 5th at 10:00.",
		"the writer only as the object": "Robin will send me the pricing by Friday.",
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			robinSeat(t, e)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			ines := employeeOf(t, e, company, "Ines Huber")
			message := captureCommitmentMessage(t, e, ines, "thread-demo", "Product demo", "Hi Ines, "+sentence, "outbound",
				extractClock.Add(-48*time.Hour))
			party(t, message, "from", &e.Rep1, nil)
			party(t, message, "to", nil, &ines)

			// The wrong answer: the sender named as promiser.
			extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), sentence, "")})

			if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
				t.Errorf("Robin's demo became %d task(s) for the sender", n)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
				t.Errorf("%d request(s) staged for a promise with no promiser, want none", n)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission
				WHERE outcome = 'promiser_not_assignable' AND NOT current_work`); n != 1 {
				t.Errorf("the promise is recorded %d time(s) on the message, want once", n)
			}
		})
	}
}

func TestTheSendersOwnPromiseStillFilesTheirTask(t *testing.T) {
	for name, sentence := range map[string]string{
		"the writer as the subject beside the colleague": "I'll run the product demo with Robin on the 5th at 10:00.",
		"German writer as the subject":                   "Ich schicke Ihnen die Unterlagen mit Robin bis Freitag.",
		"no colleague named":                             "Product demo on the 5th at 10:00, online.",
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			robinSeat(t, e)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			ines := employeeOf(t, e, company, "Ines Huber")
			message := captureCommitmentMessage(t, e, ines, "thread-own", "Product demo", "Hi Ines, "+sentence, "outbound",
				extractClock.Add(-48*time.Hour))
			party(t, message, "from", &e.Rep1, nil)
			party(t, message, "to", nil, &ines)

			extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), sentence, "")})

			if got := e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ','), '') FROM activity WHERE kind = 'task'`); got != e.Rep1.String() {
				t.Errorf("the sender's own promise filed tasks for %q, want one for the sender", got)
			}
		})
	}
}

// restatedReply is the model naming a promise and a later message renewing it.
func restatedReply(t *testing.T, original, renewed ids.UUID, promiser, quote, renewedQuote string) string {
	t.Helper()
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "promiser_ref": promiser, "message_id": original.String(), "quote": quote,
		"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
		"commitment": OpenCommitmentMeaning("2026-06-05", renewedQuote),
		"summary":    "Send the pricing", "due_date": "2026-06-05", "confidence": 0.95,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(reply)
}

func TestARestatementInAnAbsentColleaguesWordsRenewsNothing(t *testing.T) {
	e := Setup(t)
	robinSeat(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	quote := "I'll send the pricing."
	original := captureCommitmentMessage(t, e, ines, "thread-renewed-robin", "Pricing", "Hi Ines, "+quote, "outbound", extractClock.Add(-72*time.Hour))
	party(t, original, "from", &e.Rep1, nil)
	party(t, original, "to", nil, &ines)
	renewedQuote := "Robin sends the pricing on Friday."
	renewed := captureCommitmentMessage(t, e, ines, "thread-renewed-robin", "Re: Pricing", "Hi Ines, "+renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
	party(t, renewed, "from", &e.Rep1, nil)
	party(t, renewed, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: restatedReply(t, original, renewed, "seat:"+e.Rep1.String(), quote, renewedQuote)})

	if got := e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ','), '') FROM activity WHERE kind = 'task'`); got != e.Rep1.String() {
		t.Fatalf("the sender's own promise filed tasks for %q, want one for the sender", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE renewed_by = $1 OR renewed_at IS NOT NULL`, renewed); got != 0 {
		t.Errorf("Robin's sentence renewed the sender's promise (%d admission rows record a renewal)", got)
	}
}

func TestAnAbsentColleaguesPromiseWithARestatementIsStillRecorded(t *testing.T) {
	e := Setup(t)
	robinSeat(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	quote := "Robin sends the pricing."
	original := captureCommitmentMessage(t, e, ines, "thread-robin-twice", "Pricing", "Hi Ines, "+quote, "outbound", extractClock.Add(-72*time.Hour))
	party(t, original, "from", &e.Rep1, nil)
	party(t, original, "to", nil, &ines)
	renewedQuote := "Robin sends the pricing on Friday."
	renewed := captureCommitmentMessage(t, e, ines, "thread-robin-twice", "Re: Pricing", "Hi Ines, "+renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
	party(t, renewed, "from", &e.Rep1, nil)
	party(t, renewed, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: restatedReply(t, original, renewed, "seat:"+e.Rep1.String(), quote, renewedQuote)})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("Robin's promise became %d task(s) for the sender", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission
		WHERE outcome = 'promiser_not_assignable' AND NOT current_work`); n != 1 {
		t.Errorf("the promise is recorded %d time(s), want once", n)
	}
}
