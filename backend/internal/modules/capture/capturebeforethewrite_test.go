// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// joiningSink is a sink whose ThreadJoiner is complete enough to get past the
// first guard, so a case can reach the ones after it. Its Neighbours would fail
// the test if called, which is what makes "skipped" an assertion rather than an
// absence: every case here must decide WITHOUT asking the database.
func joiningSink(t *testing.T) *Sink {
	t.Helper()
	return (&Sink{}).WithThreadJoin(ThreadJoiner{
		RecordReferences: func(context.Context, pgx.Tx, ids.ActivityID, []string) error { return nil },
		Neighbours: func(context.Context, pgx.Tx, ids.ActivityID, string, []string) ([]ids.ActivityID, error) {
			t.Error("the neighbour probe ran for a record that cannot merge — it costs a query per capture")
			return nil, nil
		},
		Earliest: func(context.Context, pgx.Tx, []string) (string, error) { return "", nil },
		Merge:    func(context.Context, pgx.Tx, string, string) error { return nil },
	})
}

// The lock is off the common path, which is the claim takeMergeLockFirst's comment
// makes and the reason the probe is there at all. A workspace-wide lock taken by
// every capture would serialise the whole sync.
//
// An email is NOT here, even one carrying no References. A stored reply can point
// at it, which makes it a merge candidate its own header cannot reveal, so the
// probe has to ask — see takeMergeLockFirst.
//
// The case passes a nil transaction on purpose: a decision that reaches for one
// has already failed, and the nil would panic rather than pass quietly.
func TestTheMergeLockIsSkippedByACaptureThatCannotMerge(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		record connector.NormalizedRecord
	}{
		{
			"a chat, whatever it quotes",
			connector.NormalizedRecord{
				NaturalKey: connector.NaturalKey{SourceSystem: "slack", SourceID: "m1"},
				ReplyTo:    []string{"whatever@example.test"},
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := joiningSink(t).takeMergeLockFirst(context.Background(), nil, c.record); err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
		})
	}
}

// And a sink with no thread join at all does nothing, because there is no merge
// for a lock to order against.
func TestASinkThatDoesNotJoinThreadsTakesNoMergeLock(t *testing.T) {
	t.Parallel()

	reply := connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m3"},
		ReplyTo:    []string{"parent@example.test"},
	}
	if err := (&Sink{}).takeMergeLockFirst(context.Background(), nil, reply); err != nil {
		t.Errorf("a sink without a thread joiner: %v", err)
	}
}

// A meeting an invitation already filed is TAKEN OVER, not minted again. The
// resolver answers for the row the invitation created, and captureActivity returns
// it before anything is written — which is the whole reason this lives beside the
// merge lock rather than inside the write.
func TestAMeetingAnInvitationAlreadyFiledIsTakenOver(t *testing.T) {
	t.Parallel()

	filed := ids.ActivityID{UUID: ids.NewV7()}
	sink := (&Sink{}).WithCalendarInvitations(
		func(context.Context, pgx.Tx, connector.NaturalKey, []byte) (ids.ActivityID, bool, error) {
			return filed, true, nil
		})

	ref, found, err := sink.invitationAlreadyFiled(context.Background(), nil,
		connector.NormalizedRecord{NaturalKey: connector.NaturalKey{SourceSystem: "google", SourceID: "evt"}},
		ActivityFields{Kind: meetingKind})
	if err != nil {
		t.Fatalf("resolving an invitation: %v", err)
	}
	if !found {
		t.Fatal("an invitation the resolver answered for was not reported as filed, so the capture " +
			"would mint a second row for the same meeting")
	}
	if ref.ID != filed.UUID {
		t.Errorf("took over %s, want the invitation's own row %s", ref.ID, filed.UUID)
	}
	if ref.Type != datasource.EntityActivity {
		t.Errorf("the taken-over reference is a %q, want an activity", ref.Type)
	}
}

// And a record that is not a meeting never asks, whatever a resolver is bound.
func TestANonMeetingNeverAsksTheInvitationResolver(t *testing.T) {
	t.Parallel()

	sink := (&Sink{}).WithCalendarInvitations(
		func(context.Context, pgx.Tx, connector.NaturalKey, []byte) (ids.ActivityID, bool, error) {
			t.Error("the invitation resolver ran for a record that is not a meeting")
			return ids.ActivityID{}, false, nil
		})
	if _, found, err := sink.invitationAlreadyFiled(context.Background(), nil,
		connector.NormalizedRecord{}, ActivityFields{Kind: "email"}); err != nil || found {
		t.Errorf("found = %t, err = %v; want a record that is not a meeting to resolve nothing", found, err)
	}
}

// countingTx records whether the lock statement was reached. The probe's decision
// is invisible otherwise: "no lock" and "a lock nobody looked for" read the same.
type countingTx struct {
	pgx.Tx
	locks *int
}

func (c countingTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "pg_advisory_xact_lock") {
		*c.locks++
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

// probingSink answers the neighbour question with whatever a case needs, which is
// what lets an EMAIL case reach the decision the source-system guard would
// otherwise short-circuit.
func probingSink(links []ids.ActivityID, probeErr error) *Sink {
	return (&Sink{}).WithThreadJoin(ThreadJoiner{
		RecordReferences: func(context.Context, pgx.Tx, ids.ActivityID, []string) error { return nil },
		Neighbours: func(context.Context, pgx.Tx, ids.ActivityID, string, []string) ([]ids.ActivityID, error) {
			return links, probeErr
		},
		Earliest: func(context.Context, pgx.Tx, []string) (string, error) { return "", nil },
		Merge:    func(context.Context, pgx.Tx, string, string) error { return nil },
	})
}

func anEmail() connector.NormalizedRecord {
	return connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-probe"},
	}
}

// The email branches the fix rests on, which the source-system guard cannot reach.
//
// An email with no stored link takes no lock — that is what keeps a workspace-wide
// lock off the common path once the ReplyTo gate is gone, and it is the only thing
// left doing so. An email WITH one takes it, before anything is written.
func TestAnEmailTakesTheLockOnlyWhenSomethingStoredLinksToIt(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		links []ids.ActivityID
		want  int
	}{
		"nothing stored links to it": {links: nil, want: 0},
		"one stored message does":    {links: []ids.ActivityID{{UUID: ids.NewV7()}}, want: 1},
	} {
		locks := 0
		if err := probingSink(c.links, nil).takeMergeLockFirst(
			context.Background(), countingTx{locks: &locks}, anEmail()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if locks != c.want {
			t.Errorf("%s: took the lock %d time(s), want %d", name, locks, c.want)
		}
	}
}

// And a probe that fails stops the capture rather than carrying on unlocked: a
// capture that cannot tell whether it may merge must not proceed to write a row it
// would then lock out of order.
func TestAFailingNeighbourProbeStopsTheCaptureBeforeItLocks(t *testing.T) {
	t.Parallel()

	locks := 0
	err := probingSink(nil, errors.New("the database refused")).takeMergeLockFirst(
		context.Background(), countingTx{locks: &locks}, anEmail())
	if err == nil {
		t.Error("the neighbour probe failed and the capture carried on, so whether this message " +
			"may merge was never established")
	}
	if locks != 0 {
		t.Errorf("the lock was taken %d time(s) after the probe failed, want 0", locks)
	}
}
