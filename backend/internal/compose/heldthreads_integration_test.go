// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The held-threads list, and the one property that decides whether it may
// exist at all: it answers about the caller's own mailbox and about nothing
// else. The rows name threads a classifier judged legal, personnel or personal,
// so a list that reached a colleague's would disclose precisely what holding
// them prevents.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheHeldThreadsListIsOneSeatsOwn(t *testing.T) {
	e := integration.Setup(t)
	seedVerdict(t, e, e.Rep1, "thread-mine", "held", "legal", 1)
	seedVerdict(t, e, e.Rep2, "thread-theirs", "held", "personnel", 1)

	mine := heldThreads(t, e, e.Rep1)
	if len(mine) != 1 {
		t.Fatalf("Rep1 sees %d threads, want 1", len(mine))
	}
	if mine[0].ThreadKey != "thread-mine" {
		t.Errorf("Rep1 sees %q — a colleague's held thread reached their list, "+
			"which discloses exactly what holding it prevents", mine[0].ThreadKey)
	}
	// The kind is the disclosure that matters most: "personnel" says somebody
	// is in an HR process, and it must not travel even as a bare word.
	for _, tr := range mine {
		if tr.Kind == "personnel" {
			t.Errorf("Rep1 read a colleague's verdict kind %q", tr.Kind)
		}
	}
}

// A thread the classifier opened is not held, so it is not on a page that
// answers "what is not visible to my colleagues".
func TestAClearedThreadIsNotHeld(t *testing.T) {
	e := integration.Setup(t)
	seedVerdict(t, e, e.Rep1, "thread-open", "cleared", "ordinary", 1)
	seedVerdict(t, e, e.Rep1, "thread-shared", "shared_by_owner", "", 1)
	seedVerdict(t, e, e.Rep1, "thread-unsure", "unsure", "", 2)

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads held, want 1 — only the unsure one withholds", len(got))
	}
	// `unsure` is the case worth pinning: a thread the model could not judge
	// withholds exactly like one it judged legal, and an owner who is not shown
	// it cannot release it.
	if got[0].ThreadKey != "thread-unsure" {
		t.Errorf("held thread is %q, want thread-unsure", got[0].ThreadKey)
	}
}

// Pending first, because during an outage those are the rows nobody has
// decided — burying them under decided ones is what makes an owner scroll to
// find the work.
func TestPendingThreadsComeFirst(t *testing.T) {
	e := integration.Setup(t)
	seedVerdict(t, e, e.Rep1, "thread-decided", "held", "legal", 1)
	seedVerdict(t, e, e.Rep1, "thread-waiting", "pending", "", 3)

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 2 {
		t.Fatalf("%d threads, want 2", len(got))
	}
	if got[0].ThreadKey != "thread-waiting" {
		t.Errorf("first row is %q, want thread-waiting — pending rows lead", got[0].ThreadKey)
	}
	if !got[0].Pending() {
		t.Error("the pending row does not report itself pending, so a client cannot style it")
	}
	if got[1].Pending() {
		t.Error("a decided row reports itself pending")
	}
	// The outage signal travels: an owner watching attempts stop climbing is
	// how they tell a stalled model from a slow one.
	if got[0].Attempts != 3 {
		t.Errorf("attempts = %d, want 3", got[0].Attempts)
	}
}

// The ledger outlives the message it was raised about — first_activity_id is
// ON DELETE SET NULL, because losing the verdict would re-open a thread a
// classifier already held. An inner join would drop exactly those rows.
func TestAThreadWhoseMessageWasErasedIsStillListed(t *testing.T) {
	e := integration.Setup(t)
	seedVerdictOrphan(t, e, e.Rep1, "thread-orphan", "held", "personal", 1)

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1 — a verdict with no activity still holds", len(got))
	}
	if got[0].HasActivity {
		t.Error("an orphaned verdict reports an activity it does not have — " +
			"which the card draws as a message with a blank subject line")
	}
	if got[0].Kind != "personal" {
		t.Errorf("kind = %q, want personal — the verdict survives its evidence", got[0].Kind)
	}
}

// The subject is activity CONTENT, and it reaches the reader through the same
// clause every other reader of that table composes.
//
// This is the caller's OWN mailbox, so the clause admits them and the subject
// arrives — which is exactly why the rule is easy to leave out and why nothing
// on the happy path notices. What it buys is the row the clause refuses: a
// verdict whose activity the caller may not read the content of is still listed
// (they are holding it, and must be able to release it) and carries no subject.
func TestAHeldThreadCarriesItsSubjectOnlyWhenTheReaderMayReadIt(t *testing.T) {
	e := integration.Setup(t)
	seedVerdict(t, e, e.Rep1, "thread-mine", "held", "legal", 1)

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1", len(got))
	}
	if !got[0].HasActivity {
		t.Fatal("the reader's own message did not reach them — the content clause " +
			"is refusing a row it should admit")
	}
	if got[0].Subject != "Subject of thread-mine" {
		t.Errorf("subject = %q, want the seeded one", got[0].Subject)
	}
}

// A message a colleague imported and holds, whose verdict row is the CALLER's.
// The ledger row is theirs to see and release; the message's content is not
// theirs to read, and the two answers are separate.
func TestAHeldThreadWithholdsAMessageTheReaderIsNotOn(t *testing.T) {
	e := integration.Setup(t)
	var activityID ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		activityID = ids.NewV7()
		// Captured by SOMEBODY ELSE and held to its participants, so the
		// caller is outside the audience.
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Entwurf Aufhebungsvertrag', now(), 'gmail', $2, 'participants')`,
			activityID, "human:"+e.Rep2.String())
		return err
	}); err != nil {
		t.Fatalf("seeding a colleague's message: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, attempts, first_activity_id)
			VALUES ('thread-colleagues', $1, 'held', 'personnel', 1, $2)`, e.Rep1, activityID)
		return err
	}); err != nil {
		t.Fatalf("seeding the verdict: %v", err)
	}

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1 — the row is the caller's to see", len(got))
	}
	if got[0].Subject == "Entwurf Aufhebungsvertrag" {
		t.Error("a message the caller is not on handed them its subject — " +
			"the content clause is not composed")
	}
	// And no id either. The id is what a client OPENS, so handing one out for
	// a message whose subject is withheld would route the reader straight to
	// the words the line above just refused them. It is read from the joined
	// activity row for exactly this reason — the ledger's own
	// first_activity_id carries no content gate.
	if got[0].ActivityID != nil {
		t.Errorf("a message the caller is not on handed them its id (%v) — "+
			"the id is read from the ledger rather than the gated join",
			*got[0].ActivityID)
	}
}

// The id a client opens is present for a message the reader may read, and the
// admission case is asserted as hard as the refusal: a list that handed out no
// id at all would pass every refusal above while opening nothing.
func TestAHeldThreadCarriesTheMessageItsReaderMayOpen(t *testing.T) {
	e := integration.Setup(t)
	activityID := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Rechnung Q3', now(), 'gmail', $2, 'workspace')`,
			activityID, "human:"+e.Rep1.String())
		return err
	}); err != nil {
		t.Fatalf("seeding the message: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, attempts, first_activity_id)
			VALUES ('thread-openable', $1, 'held', 'financial_corporate', 1, $2)`,
			e.Rep1, activityID)
		return err
	}); err != nil {
		t.Fatalf("seeding the verdict: %v", err)
	}

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1", len(got))
	}
	if got[0].ActivityID == nil {
		t.Fatal("a held thread whose message the reader may read carried no id; " +
			"there is nothing for the drawer to open")
	}
	if *got[0].ActivityID != activityID {
		t.Errorf("the row names activity %v, want the message that opened the thread %v",
			*got[0].ActivityID, activityID)
	}
}

func heldThreads(t *testing.T, e *integration.Env, user ids.UUID) []capture.HeldThread {
	t.Helper()
	got, err := capture.HeldThreadsFor(purgeCtx(e, user), InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("listing held threads: %v", err)
	}
	return got
}

// seedVerdict writes one ledger row and the message it was raised about. The
// engine that normally writes these needs a model and a whole capture to run;
// what these cases are about is the READ, and a row is the honest input to that.
//
// The ACTIVITY is not optional here even though the column is. Every case in
// this file seeded a verdict with a null first_activity_id at first, which left
// the activity join — and the content clause on it — unexercised by the whole
// suite: the subject leaked without an audience test and every case stayed
// green. A fixture that omits the field models a different record.
func seedVerdict(t *testing.T, e *integration.Env, user ids.UUID, threadKey, status, kind string, attempts int) {
	t.Helper()
	seedVerdictWithActivity(t, e, user, threadKey, status, kind, attempts, true)
}

// seedVerdictOrphan is the verdict whose message was erased while it stood,
// which is the state ON DELETE SET NULL exists to produce.
func seedVerdictOrphan(t *testing.T, e *integration.Env, user ids.UUID, threadKey, status, kind string, attempts int) {
	t.Helper()
	seedVerdictWithActivity(t, e, user, threadKey, status, kind, attempts, false)
}

func seedVerdictWithActivity(t *testing.T, e *integration.Env, user ids.UUID,
	threadKey, status, kind string, attempts int, withActivity bool,
) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var activityID *ids.UUID
		if withActivity {
			id := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
				VALUES ($1, 'email', $2, now(), 'gmail', $3, 'participants')`,
				id, "Subject of "+threadKey, "human:"+user.String()); err != nil {
				return err
			}
			activityID = &id
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, attempts, first_activity_id)
			VALUES ($1, $2, $3, nullif($4, ''), $5, $6)`,
			threadKey, user, status, kind, attempts, activityID)
		return err
	}); err != nil {
		t.Fatalf("seeding a verdict: %v", err)
	}
}
