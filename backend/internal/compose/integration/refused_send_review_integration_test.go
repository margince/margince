// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a rep is left with when the engine refuses their message.
//
// Until this, the answer was 409 consent_not_granted and nothing else. The
// refusal rolled its whole transaction back, so nothing durable said what was
// refused, which recipient it was refused for, or what would change the answer.
// The rep pressed send, read a code, and had nowhere to go.
//
// A review is what the refusal leaves behind now. These tests hold three
// things: the row exists after the send that failed, it names the recipient and
// the reason, and the refusal itself carries the reference so the rep can find
// it without a queue to search.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// reviewRow is what these tests read back.
type reviewRow struct {
	id         string
	state      string
	reasonCode string
	refusals   string
}

// openReviews reads every live review in the installation.
func openReviews(t *testing.T, e *apptest.AppEnv) []reviewRow {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT id::text, state, reason_code, refusals::text
		  FROM communication_review
		 WHERE resolved_at IS NULL
		 ORDER BY opened_at`)
	if err != nil {
		t.Fatalf("reading the reviews: %v", err)
	}
	defer rows.Close()
	var out []reviewRow
	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.id, &r.state, &r.reasonCode, &r.refusals); err != nil {
			t.Fatalf("scanning a review: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the reviews: %v", err)
	}
	return out
}

// heldRow is one message frozen by a refusal, as these tests read it back.
type heldRow struct {
	id      string
	reason  string
	subject string
	payload string
}

// heldSends reads every message this installation is holding.
func heldSends(t *testing.T, e *apptest.AppEnv) []heldRow {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT id::text, held_reason, coalesce(payload->>'subject', ''), payload::text
		  FROM scheduled_send
		 WHERE status = 'held'
		 ORDER BY created_at`)
	if err != nil {
		t.Fatalf("reading the held messages: %v", err)
	}
	defer rows.Close()
	var out []heldRow
	for rows.Next() {
		var r heldRow
		if err := rows.Scan(&r.id, &r.reason, &r.subject, &r.payload); err != nil {
			t.Fatalf("scanning a held message: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the held messages: %v", err)
	}
	return out
}

// THE REFUSED MESSAGE SURVIVES, AND THE REVIEW POINTS AT IT.
//
// This is the slice. Before it, the refusal rolled its transaction back and the
// message went with it: the review named no intent, so there was nothing to
// resume and nothing for a directed send to execute on. A rep who opened their
// review found a record of what happened and no way to act on it.
//
// Now the exact bytes are frozen into a held scheduled_send before the review
// is opened, and the review binds to that row.
func TestARefusedSendHoldsItsMessageAndTheReviewBindsToIt(t *testing.T) {
	c := setupConsent(t)

	if held := heldSends(t, c.AppEnv); len(held) != 0 {
		t.Fatalf("%d held message(s) before anything was refused, want none", len(held))
	}

	status, _ := c.send(t, "marketing_email")
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}

	held := heldSends(t, c.AppEnv)
	if len(held) != 1 {
		t.Fatalf("%d held message(s) after a refused send, want 1 — the message rolled back "+
			"with its transaction and there is nothing to resume", len(held))
	}
	// THE REASON NAMES WHAT HAPPENED. "held" alone tells a rep nothing they can
	// act on, which is why the column has always been required for a hold.
	if held[0].reason != "send_refused" {
		t.Errorf("the held message reads reason %q, want send_refused", held[0].reason)
	}
	// THE MESSAGE ITSELF, not a summary of it. What is frozen is the rep's
	// input — subject, body, recipients, attachments, the claimed purpose —
	// which is what scheduling freezes too; the signature and footer are
	// re-derived at fire. A row holding only a reason code would leave the rep
	// retyping the message they had already written.
	if !strings.Contains(held[0].payload, "subject@consent.test") {
		t.Errorf("the held message does not name its recipient: %s", held[0].payload)
	}
	if held[0].subject == "" {
		t.Error("the held message carries no subject, so what was frozen is not the message")
	}

	var boundTo string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT coalesce(delivery_intent_id::text, '')
		  FROM communication_review WHERE resolved_at IS NULL`).Scan(&boundTo); err != nil {
		t.Fatalf("reading the review's intent: %v", err)
	}
	if boundTo != held[0].id {
		t.Errorf("the review binds to intent %q, want the held message %q — a review naming no "+
			"intent is the gap that leaves a directed send nothing to execute on",
			boundTo, held[0].id)
	}
}

// NO TIMER IS ARMED ON A HELD MESSAGE. A scheduled send promises to go out at a
// moment; this is the opposite — a message that must not go anywhere until a
// human decides something. A timer would fire it.
func TestAHeldRefusalIsNotWaitingToBeSent(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	held := heldSends(t, c.AppEnv)
	if len(held) != 1 {
		t.Fatalf("%d held message(s), want 1", len(held))
	}

	// EVERY job kind, not the fire kind alone. Naming one kind is how this
	// test was wrong the first time: it asked about a kind that does not
	// exist, so an armed timer would have left it green. Any job at all
	// carrying this row's id is a job that intends to do something with a
	// message a human has not decided about yet.
	var timers int
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM river_job WHERE args::text LIKE '%' || $1 || '%'`,
		held[0].id).Scan(&timers); err != nil {
		t.Fatalf("counting the timers: %v", err)
	}
	if timers != 0 {
		t.Errorf("%d job(s) armed on a held refusal — a message stopped for a human decision "+
			"would send itself", timers)
	}
	// And the row is not in a state anything fires from. The fire path takes
	// rows reading 'scheduled'; a held one is passed over.
	var status string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT status FROM scheduled_send WHERE id = $1`, held[0].id).Scan(&status); err != nil {
		t.Fatalf("reading the held message's status: %v", err)
	}
	if status != "held" {
		t.Errorf("the refused message reads %q, want held — anything else is a state the fire "+
			"path will pick up", status)
	}
}

// ANOTHER SEAT'S REFUSAL IS NOT MINE TO REUSE. A held row names who would fire
// it — whose mailbox the message leaves from and whose signature it carries —
// so reusing one rep's held message for another rep's send would bind the
// second rep's review to a message that goes out as the first rep.
//
// Driven by planting the other seat's row directly. A second signed-in session
// is a fixture this file does not have, and the rule under test is the twin
// query's seat predicate rather than anything the second rep's request does:
// what has to be true is that a held row this seat does not own is not a row
// this seat's refusal reuses.
func TestOneRepsHeldMessageIsNotReusedForAnother(t *testing.T) {
	c := setupConsent(t)

	// The other seat's row is planted FIRST, and it is the only held row in the
	// installation when this seat presses send. So if the twin read stopped
	// keying on the seat, this is the row it would find and reuse — the test
	// would then see one held message where it wants two, and the refusal's
	// review would be bound to a message that fires from somebody else's
	// mailbox.
	var other string
	if err := c.Owner.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name, status, seat_type, password_hash)
		VALUES ('other@consent.test', 'Other Rep', 'active', 'full', 'x')
		RETURNING id::text`).Scan(&other); err != nil {
		t.Fatalf("creating the other seat: %v", err)
	}

	// Their message is built by letting THIS seat be refused once, copying the
	// frozen row onto the other seat, and clearing this seat's own — which
	// leaves a held message identical in every column a payload comparison can
	// see, owned by somebody else.
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	seeded := heldSends(t, c.AppEnv)
	if len(seeded) != 1 {
		t.Fatalf("%d held message(s) to copy from, want 1", len(seeded))
	}
	if _, err := c.Owner.Exec(context.Background(), `
		INSERT INTO scheduled_send
		  (id, status, held_reason, scheduled_at, scheduled_tz, origin_kind,
		   anchor_activity_id, origin_links, also_links, payload, payload_version,
		   scheduled_by, principal_kind)
		SELECT gen_random_uuid(), status, held_reason, scheduled_at, scheduled_tz,
		       origin_kind, anchor_activity_id, origin_links, also_links, payload,
		       payload_version, $1, principal_kind
		  FROM scheduled_send WHERE id = $2`, other, seeded[0].id); err != nil {
		t.Fatalf("planting the other seat's held message: %v", err)
	}
	// This seat's own row and the review bound to it go, so what remains is one
	// held message belonging to somebody else.
	if _, err := c.Owner.Exec(context.Background(),
		`DELETE FROM scheduled_send WHERE id = $1`, seeded[0].id); err != nil {
		t.Fatalf("clearing this seat's held message: %v", err)
	}
	if held := heldSends(t, c.AppEnv); len(held) != 1 {
		t.Fatalf("%d held message(s) before this seat presses send, want 1 (the other seat's)",
			len(held))
	}

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("this seat's marketing send → %d, want 409", status)
	}

	held := heldSends(t, c.AppEnv)
	if len(held) != 2 {
		t.Fatalf("%d held message(s) after this seat was refused, want 2 — this seat's refusal "+
			"reused the other seat's message, which would fire from their mailbox over their "+
			"signature", len(held))
	}
	var seats int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(DISTINCT scheduled_by) FROM scheduled_send WHERE status = 'held'`).
		Scan(&seats); err != nil {
		t.Fatalf("counting the seats holding a message: %v", err)
	}
	if seats != 2 {
		t.Errorf("%d distinct seat(s) hold the held messages, want 2 — a held message that "+
			"names the wrong seat sends over the wrong signature", seats)
	}
	// And the review this seat's refusal opened names THIS seat's message.
	var boundSeat string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT s.scheduled_by::text
		  FROM communication_review r JOIN scheduled_send s ON s.id = r.delivery_intent_id
		 WHERE r.resolved_at IS NULL`).Scan(&boundSeat); err != nil {
		t.Fatalf("reading the seat behind the review's intent: %v", err)
	}
	if boundSeat == other {
		t.Error("the review binds to the OTHER seat's held message — resuming it would send " +
			"this rep's refusal from somebody else's mailbox")
	}
}

// A REFUSED SEND LEAVES A REVIEW. This is the slice: the message stopped, and
// there is now something to look at rather than a code in a response body.
func TestARefusedSendLeavesAReviewBehind(t *testing.T) {
	c := setupConsent(t)

	if reviews := openReviews(t, c.AppEnv); len(reviews) != 0 {
		t.Fatalf("%d review(s) before anything was refused, want none", len(reviews))
	}

	status, code := c.send(t, "marketing_email")
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d %q, want 409", status, code)
	}

	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s) after a refused send, want 1 — the rep read a code and the "+
			"installation kept no record of what it refused or for whom", len(reviews))
	}
	// The RECIPIENT is named. A reason code alone says a message was refused
	// and not who for, which is the question the rep is actually asking.
	if !strings.Contains(reviews[0].refusals, "subject@consent.test") {
		t.Errorf("the review names no recipient: %s", reviews[0].refusals)
	}
	if reviews[0].reasonCode == "" {
		t.Error("the review carries no reason code, so nothing says why the send stopped")
	}
}

// THE REFUSAL CARRIES THE REFERENCE. A review nobody can find is a row in a
// table, not a piece of work — the rep has no queue to search and no id to
// quote, so the reference has to travel with the answer they already get.
func TestTheRefusalNamesTheReviewItOpened(t *testing.T) {
	c := setupConsent(t)

	var problem struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
	}, nil, &problem)
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}

	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}
	answer := problem.Message + " " + problem.Detail
	if !strings.Contains(answer, reviews[0].id) {
		t.Errorf("the refusal answered %q and never named review %s — the rep has a row they "+
			"cannot find", answer, reviews[0].id)
	}
}

// THE SAME MESSAGE PRESSED TWICE IS ONE PIECE OF WORK. A rep reads the refusal
// and presses send again, which is the ordinary thing to do. Two held copies
// and two reviews would put one message in front of somebody twice, and
// resolving either would leave the other pointing at a message already dealt
// with.
//
// This rule replaces the one that stood before refusals held their message: an
// unbound review had no intent to conflict on, so every attempt was its own
// row. Now the second press finds the standing held row, reuses it, and the
// unique index on the live intent updates the review in place.
func TestPressingSendTwiceHoldsOneMessageAndOneReview(t *testing.T) {
	c := setupConsent(t)

	for range 2 {
		if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
			t.Fatalf("marketing send with no grant → %d, want 409", status)
		}
	}
	if reviews := openReviews(t, c.AppEnv); len(reviews) != 1 {
		t.Errorf("%d review(s) after pressing send twice on one message, want 1 — the second "+
			"press reuses the held row, and the live-intent index updates the standing review",
			len(reviews))
	}
	if held := heldSends(t, c.AppEnv); len(held) != 1 {
		t.Errorf("%d held message(s) after pressing send twice, want 1 — a rep who tries again "+
			"should not accumulate copies of one message", len(held))
	}
}

// A DIFFERENT MESSAGE IS DIFFERENT WORK. The reuse above keys on the frozen
// message, so a changed subject line is a new question the engine was asked and
// gets its own held row and its own review.
//
// Without this the reuse would be keyed on something looser — the seat, the
// recipient — and a rep who fixed their message would find the review still
// showing the one they had replaced.
func TestAChangedMessageIsItsOwnHeldSendAndReview(t *testing.T) {
	c := setupConsent(t)

	for _, subject := range []string{"First attempt", "Second, reworded"} {
		status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
			"subject": subject, "body": "answer",
			"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
		}, nil, nil)
		if status != http.StatusConflict {
			t.Fatalf("marketing send with no grant → %d, want 409", status)
		}
	}
	if held := heldSends(t, c.AppEnv); len(held) != 2 {
		t.Errorf("%d held message(s) for two different messages, want 2 — reusing a row across "+
			"a reworded message would hand the rep back the draft they replaced", len(held))
	}
	if reviews := openReviews(t, c.AppEnv); len(reviews) != 2 {
		t.Errorf("%d review(s) for two different messages, want 2", len(reviews))
	}
}

// AN ERASURE CLEARS THE SUBJECT FROM A REVIEW. The row holds the addresses a
// message was refused for, which is exactly the material Art. 17 destroys — and
// unlike a decision beside it, a review is not accountability evidence: it
// records a message that never went, so there is nothing to answer for.
//
// The row SURVIVES with its refusals emptied rather than being deleted. The
// work may still be in front of a human, and a row vanishing under them leaves
// a queue pointing at nothing.
func TestErasingASubjectClearsThemFromARefusedSendReview(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 || !strings.Contains(reviews[0].refusals, "subject@consent.test") {
		t.Fatalf("no review naming the subject to erase from: %+v", reviews)
	}

	var personID string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT p.id::text FROM person p
		  JOIN person_email e ON e.person_id = p.id
		 WHERE lower(e.email) = 'subject@consent.test'`).Scan(&personID); err != nil {
		t.Fatalf("finding the subject: %v", err)
	}
	// THROUGH THE RIGHTS CASE, which is how an erasure actually happens here: a
	// case is opened naming the subject and fulfilling it runs the eraser. That
	// is the production route, so this proves the review is cleared by the path
	// an operator takes rather than by one a test invented.
	var opened struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": personID,
		"due_at": "2027-01-01T00:00:00Z",
	}, nil, &opened); status != http.StatusCreated {
		t.Fatalf("opening the erasure case → %d", status)
	}
	if status := c.Call(t, "PATCH", "/v1/data-subject-requests/"+opened.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased on request",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure → %d", status)
	}

	after := openReviews(t, c.AppEnv)
	if len(after) != 1 {
		t.Fatalf("%d review(s) after the erasure, want the row to survive with its refusals "+
			"emptied — a queue pointing at a deleted row shows a human nothing", len(after))
	}
	if strings.Contains(after[0].refusals, "subject@consent.test") {
		t.Errorf("the review still names the erased subject's address: %s", after[0].refusals)
	}
}

// AN ERASURE EMPTIES THE HELD MESSAGE AND CANCELS IT.
//
// The held row now carries the subject's address, their name in the body and
// the message meant for them, which is exactly what Art. 17 destroys. Emptying
// it is not enough on its own: a row still reading 'held' is a row somebody can
// resume, and resuming an emptied message would fire nothing at nobody.
//
// It also has to be cancelled for a mechanical reason worth naming. The
// state-shape CHECK reserves held_reason for holds, and the scrub clears that
// reason — so a row left at 'held' violates its own constraint and aborts the
// erasure, which is a subject's request failing on the very message they asked
// to be rid of.
func TestErasingASubjectEmptiesAndCancelsTheirHeldMessage(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	if held := heldSends(t, c.AppEnv); len(held) != 1 {
		t.Fatalf("%d held message(s) to erase from, want 1", len(held))
	}

	var personID string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT p.id::text FROM person p
		  JOIN person_email e ON e.person_id = p.id
		 WHERE lower(e.email) = 'subject@consent.test'`).Scan(&personID); err != nil {
		t.Fatalf("finding the subject: %v", err)
	}
	var opened struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": personID,
		"due_at": "2027-01-01T00:00:00Z",
	}, nil, &opened); status != http.StatusCreated {
		t.Fatalf("opening the erasure case → %d", status)
	}
	if status := c.Call(t, "PATCH", "/v1/data-subject-requests/"+opened.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased on request",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure → %d — an erasure that cannot complete because of a "+
			"held message is this slice breaking Art. 17", status)
	}

	if held := heldSends(t, c.AppEnv); len(held) != 0 {
		t.Errorf("%d message(s) still held after the erasure: %+v — the subject's message is "+
			"still resumable", len(held), held)
	}
	var status, payload string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT status, payload::text FROM scheduled_send`).Scan(&status, &payload); err != nil {
		t.Fatalf("reading the erased message: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("the erased message reads %q, want cancelled", status)
	}
	if strings.Contains(payload, "subject@consent.test") {
		t.Errorf("the erased message still names the subject: %s", payload)
	}
}

// AN ERASURE REACHES THE MESSAGE WHATEVER CASE THE REP TYPED.
//
// The held payload keeps the address as the SENDER wrote it, and the erasure's
// address list carries it as the installation STORED it. A rep who typed
// Subject@consent.test to a person recorded as subject@consent.test would
// otherwise leave that message behind — their name, their address and the words
// meant for them — after the installation had certified their data destroyed.
//
// Worse, it would stay unreachable: the erasure's address list is derived from
// person_email, which the sweep deletes, so no later erasure of the same person
// would find the row either.
func TestErasingASubjectReachesAMessageAddressedInAnotherCase(t *testing.T) {
	c := setupConsent(t)

	status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"Subject@Consent.Test"}, "consent_purpose": "marketing_email",
	}, nil, nil)
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	held := heldSends(t, c.AppEnv)
	if len(held) != 1 {
		t.Fatalf("%d held message(s), want 1", len(held))
	}
	if !strings.Contains(held[0].payload, "Subject@Consent.Test") {
		t.Fatalf("the held message does not keep the address as typed: %s", held[0].payload)
	}

	var personID string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT p.id::text FROM person p
		  JOIN person_email e ON e.person_id = p.id
		 WHERE lower(e.email) = 'subject@consent.test'`).Scan(&personID); err != nil {
		t.Fatalf("finding the subject: %v", err)
	}
	var opened struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": personID,
		"due_at": "2027-01-01T00:00:00Z",
	}, nil, &opened); status != http.StatusCreated {
		t.Fatalf("opening the erasure case → %d", status)
	}
	if status := c.Call(t, "PATCH", "/v1/data-subject-requests/"+opened.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased on request",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure → %d", status)
	}

	var payload string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT payload::text FROM scheduled_send`).Scan(&payload); err != nil {
		t.Fatalf("reading the message after the erasure: %v", err)
	}
	if strings.Contains(strings.ToLower(payload), "subject@consent.test") {
		t.Errorf("the erased subject's address survives in a held message because the rep "+
			"typed it in another case: %s", payload)
	}
}

// A REFUSAL WHOSE RECIPIENT HAS BEEN ERASED HOLDS NOTHING.
//
// The hold runs after the refused send has rolled back, so an erasure can
// finish in that gap: it sweeps the installation, certifies the subject's data
// destroyed, and commits. A hold landing afterwards would put the subject's
// address and the words meant for them back into the database — in a row no
// later erasure would find, because the address list every erasure works from
// is derived from person_email, which the sweep deletes.
//
// Driven by planting the erasure's own durable record rather than by running a
// full erasure, and that is what makes it a test of THIS rule. A real erasure
// also deletes the anchor activity, so the send answers 404 before any gate
// runs — the message never reaches the hold, and a test built that way passes
// whether the rule exists or not. What has to be true is narrower: with the
// address on the suppression list, a refusal that DOES reach the hold stores
// nothing.
//
// The refusal itself still stands. The rep is told their message was refused,
// which is true; what they do not get is a message to resume, which is right,
// because there is nobody left to send it to.
func TestARefusalWhoseRecipientWasErasedHoldsNothing(t *testing.T) {
	c := setupConsent(t)

	// erasure_suppression is what an erasure leaves behind so that re-capture
	// cannot resurrect a subject. The hash is the address, lowered and trimmed.
	if _, err := c.Owner.Exec(context.Background(), `
		INSERT INTO erasure_suppression (kind, value_hash)
		VALUES ('email', encode(sha256(lower(trim($1))::bytea), 'hex'))
		ON CONFLICT DO NOTHING`, "subject@consent.test"); err != nil {
		t.Fatalf("recording the erasure suppression: %v", err)
	}

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}

	if held := heldSends(t, c.AppEnv); len(held) != 0 {
		t.Errorf("%d message(s) held for an erased subject: %+v — the installation certified "+
			"this person's data destroyed and then stored their address and the words meant "+
			"for them again, in a row no later erasure would find", len(held), held)
	}
}

// A REFERENCE NOBODY CAN RESOLVE IS BARELY BETTER THAN AN ERROR CODE. The
// refusal answers a review id and, until this endpoint existed, nothing could
// open it: the row was written, the rep was handed the id, and there was no
// door.
func TestTheRepCanOpenTheReviewTheirRefusalNamed(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}

	var opened struct {
		ID         string `json:"id"`
		State      string `json:"state"`
		ReasonCode string `json:"reason_code"`
		Refusals   []struct {
			Address    string `json:"address"`
			ReasonCode string `json:"reason_code"`
		} `json:"refusals"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+reviews[0].id,
		nil, nil, &opened); status != http.StatusOK {
		t.Fatalf("opening the review the refusal named → %d, want 200", status)
	}
	if opened.ID != reviews[0].id {
		t.Errorf("opened review %q, want %q", opened.ID, reviews[0].id)
	}
	if len(opened.Refusals) != 1 {
		t.Fatalf("%d refusal(s) in the answer, want 1 — the rep is told a send was refused "+
			"and not who for", len(opened.Refusals))
	}
	if opened.Refusals[0].Address != "subject@consent.test" {
		t.Errorf("the answer names %q, want the recipient the send was refused for",
			opened.Refusals[0].Address)
	}
	if opened.ReasonCode == "" || opened.State == "" {
		t.Errorf("the answer carries state=%q reason=%q, want both",
			opened.State, opened.ReasonCode)
	}
}

// A REVIEW BELONGING TO SOMEBODY ELSE IS NOT FOUND, not forbidden. The row
// names the recipients of another person's message and why each was refused,
// which is a fact about those people — and "forbidden" would confirm the id
// exists, which is itself a disclosure about a message the caller may not see.
func TestAnotherSeatsReviewIsNotFound(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}

	// Reassigned to a seat this caller is not: the same shape as another rep's
	// refusal, without needing a second logged-in session to produce one.
	var other string
	if err := c.Owner.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name)
		VALUES ('other-' || gen_random_uuid() || '@consent.test', 'Other Seat')
		RETURNING id::text`).Scan(&other); err != nil {
		t.Fatalf("seeding another seat: %v", err)
	}
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_review SET initiated_by = $1 WHERE id = $2`,
		other, reviews[0].id); err != nil {
		t.Fatalf("reassigning the review: %v", err)
	}

	if status := c.Call(t, "GET", "/v1/communication-reviews/"+reviews[0].id,
		nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("another seat's review → %d, want 404 — a 403 would confirm the id exists, "+
			"which is a disclosure about a message this caller may not see", status)
	}
}
