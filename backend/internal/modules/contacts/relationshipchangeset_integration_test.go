// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// countingTx counts the statements a read sends through one transaction.
type countingTx struct {
	pgx.Tx
	statements int
}

func (c *countingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.statements++
	return c.Tx.Query(ctx, sql, args...)
}

func (c *countingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.statements++
	return c.Tx.QueryRow(ctx, sql, args...)
}

// hideInteraction takes one seeded interaction out of every fold, by archiving
// it or by narrowing its audience.
func (e *dedupeEnv) hideInteraction(t *testing.T, id ids.ActivityID, change string) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE activity SET `+change+` WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("hiding an interaction: %v", err)
	}
}

// The decay lane derives a page of candidates with one fold, however many
// there are. Each contact's fold counts its own interactions, in both windows,
// and never an archived one or one held to its participants.
func TestTheChangeFoldReadsACandidateSetInOneStatement(t *testing.T) {
	e := setupDedupe(t)
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	busy := e.seedVisibleContact(t, "Busy Fold Contact")
	lapsed := e.seedVisibleContact(t, "Lapsed Fold Contact")
	silent := e.seedVisibleContact(t, "Silent Fold Contact")
	e.seedInteraction(t, busy, day(5), "inbound")
	e.seedInteraction(t, busy, day(10), "outbound")
	e.seedInteraction(t, busy, day(60), "inbound")
	e.seedInteraction(t, busy, day(100), "outbound")
	e.hideInteraction(t, e.seedInteractionOfKind(t, busy, "email", day(3), "inbound"), "archived_at = now()")
	e.hideInteraction(t, e.seedInteractionOfKind(t, busy, "email", day(2), "outbound"), "audience = 'participants'")
	e.seedInteraction(t, lapsed, day(70), "inbound")
	set := []ids.ContactID{busy, lapsed, silent}
	touches := map[ids.ContactID]relstrength.Touches{busy: {}, lapsed: {}, silent: {}}
	at := func(ago int) *time.Time { moment := day(ago); return &moment }
	want := map[ids.ContactID]relstrength.ChangeInputs{
		busy: {
			Current:  relstrength.Inputs{LastInteraction: at(5), Count90d: 3, Inbound90d: 2, Outbound90d: 1},
			Previous: relstrength.Inputs{LastInteraction: at(60), Count90d: 2, Inbound90d: 1, Outbound90d: 1},
		},
		lapsed: {
			Current:  relstrength.Inputs{LastInteraction: at(70), Count90d: 1, Inbound90d: 1},
			Previous: relstrength.Inputs{LastInteraction: at(70), Count90d: 1, Inbound90d: 1},
		},
		silent: {},
	}

	ctx := e.as()
	statements := map[int]int{}
	var together map[ids.ContactID]relstrength.ChangeInputs
	alone := map[ids.ContactID]relstrength.ChangeInputs{}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for _, size := range []int{1, len(set)} {
			counted := &countingTx{Tx: tx}
			if _, err := e.store.RelationshipChangesForContacts(ctx, counted, set[:size], now, touches, time.UTC); err != nil {
				return err
			}
			statements[size] = counted.statements
		}
		var err error
		if together, err = changeInputsFor(ctx, tx, set, now, nil); err != nil {
			return err
		}
		for _, contact := range set {
			if alone[contact], err = changeInputs(ctx, tx, contact, now, nil); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("deriving the candidates' changes: %v", err)
	}

	if statements[len(set)] != statements[1] {
		t.Errorf("deriving %d candidates sent %d statements and one candidate %d: the fold runs once per contact",
			len(set), statements[len(set)], statements[1])
	}
	for _, contact := range set {
		if !sameChangeInputs(together[contact], want[contact]) {
			t.Errorf("contact %s folds to %+v in a set, want %+v", contact, together[contact], want[contact])
		}
		if !sameChangeInputs(alone[contact], want[contact]) {
			t.Errorf("contact %s folds to %+v alone, want %+v", contact, alone[contact], want[contact])
		}
	}
}

// sameChangeInputs compares two folds' counts and their instants.
func sameChangeInputs(got, want relstrength.ChangeInputs) bool {
	sameInstant := func(a, b *time.Time) bool { return (a == nil) == (b == nil) && (a == nil || a.Equal(*b)) }
	sameWindow := func(a, b relstrength.Inputs) bool {
		return sameInstant(a.LastInteraction, b.LastInteraction) &&
			a.Count90d == b.Count90d && a.Inbound90d == b.Inbound90d && a.Outbound90d == b.Outbound90d
	}
	return sameWindow(got.Current, want.Current) && sameWindow(got.Previous, want.Previous)
}
