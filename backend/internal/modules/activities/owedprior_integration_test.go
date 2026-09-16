// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The context block: our own last message in the thread, carried beside the one
// being judged.
//
// It is the difference between reading "Dienstag 14 Uhr würde bei uns passen" as
// a statement and reading it as an unanswered answer to a plan we sent. The
// join that finds it is the part that can be quietly wrong — matching nothing,
// matching the wrong thread, or matching every threadless row to every other —
// and none of that is visible without a database.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ourReply writes one outbound message into a thread, at a given age.
func (e *loadEnv) ourReply(t *testing.T, subject, body, threadKey string, daysAgo int) {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', $2, $3, now() - make_interval(days => $5), $4, 'seed', 'system')`,
		activity, subject, body, threadKey, daysAgo)
}

// threadKeyOf reads the key waitingFrom derived for a seeded message.
func threadKeyOf(t *testing.T, e *loadEnv, id ids.UUID) string {
	t.Helper()
	var key string
	if err := e.owner.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`, id).Scan(&key); err != nil {
		t.Fatalf("reading the thread key: %v", err)
	}
	return key
}

// The newest of our earlier messages in the thread is the one carried.
func TestTheCandidateCarriesOurLastMessageBeforeIt(t *testing.T) {
	e := setupLoad(t)
	activity := e.waitingFrom(t, "Re: the plan", "buyer@customer.test", e.buyer(t))
	thread := threadKeyOf(t, e, activity)
	e.ourReply(t, "the plan", "An early draft.", thread, 9)
	e.ourReply(t, "the plan, revised", "Here is the plan. When suits you?", thread, 4)

	prior := candidate(t, e, activity).PriorOutbound
	if prior == nil {
		t.Fatal("no earlier message was carried, so the reply is judged without what it answers")
	}
	if prior.Subject != "the plan, revised" {
		t.Errorf("carried %q, want the newest earlier message %q", prior.Subject, "the plan, revised")
	}
	if prior.Body == "" {
		t.Error("the earlier message was carried with no body, so it says nothing")
	}
	if prior.At.IsZero() {
		t.Error("the earlier message was carried with no date")
	}
}

// A message with no thread carries no context, rather than somebody else's.
//
// Thread keys are never NULL-matched: one unthreaded outbound of ours would
// otherwise become the context for every unthreaded question in the workspace.
func TestAThreadlessCandidateCarriesNoPriorMessage(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	activity := ids.NewV7()
	// Threadless, and labelled so the waiting queue admits it at all.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, capture_label, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'No thread here', now() - interval '2 days', 'meeting', 'seed', 'system')`,
		activity)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'buyer@customer.test')`, ids.NewV7(), activity)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, contact)
	// Ours, also threadless. Under a NULL-matching join this would attach.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Unrelated', 'Nothing to do with it.', now() - interval '3 days', 'seed', 'system')`,
		ids.NewV7())

	if prior := candidate(t, e, activity).PriorOutbound; prior != nil {
		t.Errorf("a threadless message was given %q as context, which belongs to no conversation of its own", prior.Subject)
	}
}

// Context never crosses a thread.
func TestAPriorMessageIsNeverReadAcrossThreads(t *testing.T) {
	e := setupLoad(t)
	activity := e.waitingFrom(t, "Re: the plan", "buyer@customer.test", e.buyer(t))
	e.ourReply(t, "another conversation entirely", "Unrelated.", "thread-somewhere-else", 3)

	if prior := candidate(t, e, activity).PriorOutbound; prior != nil {
		t.Errorf("context was taken from another thread: %q", prior.Subject)
	}
}

// A message of ours sent AFTER the one being judged is not context for it.
//
// It is also a reply, which the waiting queue reads as the conversation being
// answered — but the ordering matters on its own terms: context is what the
// customer was answering, not what we said next.
func TestALaterMessageOfOursIsNotContext(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingAgedFrom(t, "Re: the plan", "buyer@customer.test", e.buyer(t), 30)
	thread := threadKeyOf(t, e, activity)
	e.ourReply(t, "sent afterwards", "Later.", thread, 1)
	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Read through the sweep, which does not apply the queue's reply anti-join,
	// so the row is present and only the ordering decides.
	rows, _, err := store.OwedRestale(e.as(), rulesetNew, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the re-judge backlog: %v", err)
	}
	for _, row := range rows {
		if row.ID != activity {
			continue
		}
		if row.PriorOutbound != nil && row.PriorOutbound.Subject == "sent afterwards" {
			t.Error("a message we sent after the one being judged was carried as what it answers")
		}
		return
	}
	t.Fatal("the seeded message is absent from the re-judge backlog")
}

// A subjectless earlier message still carries its body.
//
// subject is nullable, and keying the context block's presence on it would drop
// the whole block for such a message — a failure that reads as the model simply
// judging worse, with nothing to point at.
func TestAPriorMessageWithNoSubjectStillCarries(t *testing.T) {
	e := setupLoad(t)
	activity := e.waitingFrom(t, "Re: the plan", "buyer@customer.test", e.buyer(t))
	thread := threadKeyOf(t, e, activity)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', NULL, 'Here is the plan. When suits you?', now() - interval '4 days', $2, 'seed', 'system')`,
		ids.NewV7(), thread)

	prior := candidate(t, e, activity).PriorOutbound
	if prior == nil {
		t.Fatal("an earlier message with no subject was dropped entirely, taking its body with it")
	}
	if prior.Body == "" {
		t.Error("the earlier message was carried with no body")
	}
}
