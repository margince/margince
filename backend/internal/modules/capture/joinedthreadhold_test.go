// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EVERY verdict, classified on purpose.
//
// holdsItsMessages decides whether a message the join filed into a conversation is
// the workspace's to read, so a verdict missing from it leaves a message open
// inside a held thread — the defect this file exists to close, reintroduced by
// omission. A table over all six is what makes adding a seventh a decision: the
// count below fails until the new one is named here, rather than defaulting to
// readable.
func TestEveryThreadVerdictSaysWhetherItHoldsItsMessages(t *testing.T) {
	t.Parallel()

	holds := map[string]bool{
		VerdictHeld:          true,
		VerdictUnsure:        true, // not yet judged is not yet shareable
		VerdictHeldByOwner:   true,
		VerdictPending:       true, // the ledger is open again; the posture holds the message meanwhile
		VerdictCleared:       false,
		VerdictSharedByOwner: false,
	}
	// bornAudience is the other reader of this same distinction, and the two must
	// agree: it decides the audience at birth and this decides it after a join.
	for status, want := range holds {
		if got := holdsItsMessages(status); got != want {
			t.Errorf("holdsItsMessages(%q) = %t, want %t", status, got, want)
		}
		audience, _ := birthDecision{verdictStatus: status}.bornAudience()
		if heldAtBirth := audience == audienceParticipants; heldAtBirth != want {
			t.Errorf("%q holds its messages after a join (%t) but is born %q — one verdict cannot "+
				"mean two things about who may read the conversation", status, want, audience)
		}
	}
	// Read from the DECLARATIONS, not counted against a number in this file. The
	// count version of this claimed adding a verdict would fail the test and it
	// would not have: a new constant elsewhere does not change the length of a map
	// written here, so the claim was false and the gap it described was open.
	for _, declared := range declaredVerdicts(t) {
		if _, classified := holds[declared]; !classified {
			t.Errorf("%s is declared in verdictinherit.go and classified nowhere here, so a message "+
				"that joins a thread carrying it reads as shareable by default", declared)
		}
	}
}

// declaredVerdicts reads the Verdict* constants out of the source that declares
// them, so the set this test checks against is the product's rather than a copy.
func declaredVerdicts(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile("verdictinherit.go")
	if err != nil {
		t.Fatalf("reading the verdict declarations: %v", err)
	}
	found := regexp.MustCompile(`(?m)^\s*Verdict[A-Za-z]*\s*=\s*"([a-z_]+)"`).FindAllStringSubmatch(string(body), -1)
	if len(found) < 6 {
		t.Fatalf("read %d verdict declarations, want at least the 6 that exist — this test is "+
			"reading less than the file and would pass over an unclassified verdict", len(found))
	}
	var out []string
	for _, m := range found {
		out = append(out, m[1])
	}
	return out
}

// A verdict this code has never heard of holds nothing, which is the safe answer
// only because the table above fails when one is added.
func TestAnUnknownVerdictHoldsNothing(t *testing.T) {
	t.Parallel()

	if holdsItsMessages("a_verdict_from_the_future") {
		t.Error("an unrecognised verdict was treated as holding, which would hide messages nobody " +
			"asked to hide")
	}
}

// A capture carrying no seat decides nothing here: the verdict being read is one
// seat's view of the conversation, and there is no seat to read it for. It returns
// before it reaches the transaction, which is why a nil one is safe to pass.
func TestACaptureWithNoSeatTakesNoJoinedHold(t *testing.T) {
	t.Parallel()

	if err := (&Sink{}).holdFromJoinedThreadTx(context.Background(), nil, ids.ActivityID{UUID: ids.NewV7()}); err != nil {
		t.Errorf("a capture naming no seat: %v", err)
	}
}

// actingAs is the seat whose view of a conversation the hold is read for.
// sinkremovemessage_test.go has the same five lines, in the EXTERNAL test package
// this file cannot reach — the function under test is unexported, so this one lives
// beside it.
func actingAs(seat ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:" + seat.String(),
		UserID: seat,
	})
}

// failingTx fails ONE named statement and passes the rest through to nothing. The
// embedded nil interface is deliberate: a method this test does not expect to be
// called panics rather than quietly answering, which is how a changed call order
// announces itself instead of passing.
type failingTx struct {
	pgx.Tx
	failOn string
	rows   int64
}

func (f failingTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, f.failOn) {
		return pgconn.CommandTag{}, errors.New("the database refused")
	}
	return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", f.rows)), nil
}

func (f failingTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return scanResult{err: errors.New("the database refused"), fail: strings.Contains(sql, f.failOn)}
}

type scanResult struct {
	err  error
	fail bool
}

func (s scanResult) Scan(dest ...any) error {
	if s.fail {
		return s.err
	}
	// The verdict this test's cases are about: held, so the write path is reached.
	if len(dest) == 1 {
		if into, ok := dest[0].(*string); ok {
			*into = VerdictHeld
		}
	}
	return nil
}

// EVERY failure on this path is reported, none swallowed. A hold that fails
// silently leaves a message readable inside a held conversation and says nothing,
// which is the one outcome worse than the bug — the capture would commit.
func TestEveryFailureHoldingAJoinedMessageIsReported(t *testing.T) {
	t.Parallel()

	ctx := actingAs(ids.NewV7())
	id := ids.ActivityID{UUID: ids.NewV7()}
	for name, tx := range map[string]pgx.Tx{
		"reading the joined thread's verdict": failingTx{failOn: "capture_thread_verdict", rows: 1},
		"recording it on the import row":      failingTx{failOn: "capture_import", rows: 1},
		"narrowing the activity":              failingTx{failOn: "UPDATE activity", rows: 1},
	} {
		if err := (&Sink{}).holdFromJoinedThreadTx(ctx, tx, id); err == nil {
			t.Errorf("%s failed and the capture carried on", name)
		}
	}

	// And a write that matched no row is a broken assumption, not a no-op: the row
	// is this capture's own and nothing else in the transaction touches it.
	if err := (&Sink{}).holdFromJoinedThreadTx(ctx, failingTx{failOn: "nothing", rows: 0}, id); err == nil {
		t.Error("the narrowing matched no row and the capture carried on, so a message stays " +
			"readable with nothing saying why")
	}
}
