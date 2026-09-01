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
	seedVerdict(t, e, e.Rep1, "thread-orphan", "held", "personal", 1)

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1 — a verdict with no activity still holds", len(got))
	}
	if got[0].Subject != "" {
		t.Errorf("subject = %q, want empty — there is no message to read one from", got[0].Subject)
	}
	if got[0].Kind != "personal" {
		t.Errorf("kind = %q, want personal — the verdict survives its evidence", got[0].Kind)
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

// seedVerdict writes one ledger row directly. The engine that normally writes
// these needs a model and a whole capture to run; what these cases are about is
// the READ, and a row is the honest input to that.
func seedVerdict(t *testing.T, e *integration.Env, user ids.UUID, threadKey, status, kind string, attempts int) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, attempts)
			VALUES ($1, $2, $3, nullif($4, ''), $5)`,
			threadKey, user, status, kind, attempts)
		return err
	}); err != nil {
		t.Fatalf("seeding a verdict: %v", err)
	}
}
