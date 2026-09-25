// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package signals

// A signal says how it was resolved, and by whom.
//
// Every human answer has appended a twelve-column row since the table existed,
// and nothing read one back: the note explaining why a signal was dismissed was
// recorded and then invisible to the next reader of the signal it was about.
//
// The rows here are written as SQL rather than through UpdateSignal because the
// projection is the subject — the INSERT is unchanged and names these same
// columns, and reaching it needs a store, a principal and a patch this case has
// no opinion about.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func resolutionEnv(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// seedSignal writes one signal and returns its id. Its resolution_state is
// `unresolved` — signal_resolved_has_entity requires a subject for a resolved
// one, and the MATCH state is a different question from the human answer this
// case is about.
func seedSignal(t *testing.T, owner *pgx.Conn) ids.SignalID {
	t.Helper()
	id := ids.New[ids.SignalKind]()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO signal (id, kind, source_channel, resolution_state, severity, summary,
		                    evidence, status, detected_at, source, captured_by)
		VALUES ($1, 'buying_intent', 'web', 'unresolved', 'info', 'They asked about pricing',
		        '[]'::jsonb, 'open', now(), 'manual', 'test')`, id); err != nil {
		t.Fatalf("seeding the signal: %v", err)
	}
	return id
}

func TestASignalCarriesTheAnswerAHumanGaveIt(t *testing.T) {
	owner := resolutionEnv(t)
	ctx := context.Background()
	id := seedSignal(t, owner)

	// Two answers, because a signal can be reopened and resolved again. The
	// read says how it stands NOW; the earlier one is the audit's to show.
	for _, answer := range []struct {
		outcome string
		note    string
		at      string
	}{
		{"resolved", "spoke to them, it was a renewal question", "2026-09-01T10:00:00Z"},
		{"dismissed", "duplicate of the one from Tuesday", "2026-09-02T10:00:00Z"},
	} {
		when, err := time.Parse(time.RFC3339, answer.at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Exec(ctx, `
			INSERT INTO signal_resolution (id, signal_id, outcome, note, source, captured_by, created_at)
			VALUES ($1, $2, $3, $4, 'manual', 'test', $5)`,
			ids.NewV7(), id, answer.outcome, answer.note, when); err != nil {
			t.Fatalf("seeding the %s answer: %v", answer.outcome, err)
		}
	}

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Read-only, so this always has work to do and its failure is real —
		// a connection that cannot roll back is one the next case inherits.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling the read back: %v", err)
		}
	}()

	sig, err := readSignal(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the signal back: %v", err)
	}
	if sig.Resolution == nil {
		t.Fatal("a signal a human answered came back with no resolution — the note saying why it " +
			"was answered that way is written on every resolution and was read by nothing")
	}
	if got := string(sig.Resolution.Outcome); got != "dismissed" {
		t.Errorf("the signal reports outcome %q, want the LATEST answer (dismissed) — a reopened "+
			"signal that reads as its first answer states a verdict that has been superseded", got)
	}
	if sig.Resolution.Note == nil || *sig.Resolution.Note != "duplicate of the one from Tuesday" {
		t.Errorf("the note came back as %v, want the latest answer's own words", sig.Resolution.Note)
	}
}

// A signal nobody has answered carries no resolution, rather than an empty one:
// the four columns arrive NULL together from the lateral join, and an object
// full of zero values would read as an answer somebody gave.
func TestAnUnansweredSignalCarriesNoResolution(t *testing.T) {
	owner := resolutionEnv(t)
	ctx := context.Background()
	id := seedSignal(t, owner)

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Read-only, so this always has work to do and its failure is real —
		// a connection that cannot roll back is one the next case inherits.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling the read back: %v", err)
		}
	}()

	sig, err := readSignal(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the signal back: %v", err)
	}
	if sig.Resolution != nil {
		t.Fatalf("an unanswered signal reports resolution %+v", *sig.Resolution)
	}
}
