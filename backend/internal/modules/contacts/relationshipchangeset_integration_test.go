// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"reflect"
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

// The decay lane derives a page of candidates with one fold, however many
// there are. Each contact's fold is the one it gets when asked alone.
func TestTheChangeFoldReadsACandidateSetInOneStatement(t *testing.T) {
	e := setupDedupe(t)
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	busy := e.seedVisibleContact(t, "Busy Fold Contact")
	lapsed := e.seedVisibleContact(t, "Lapsed Fold Contact")
	silent := e.seedVisibleContact(t, "Silent Fold Contact")
	e.seedInteraction(t, busy, now.AddDate(0, 0, -5), "inbound")
	e.seedInteraction(t, busy, now.AddDate(0, 0, -10), "outbound")
	e.seedInteraction(t, busy, now.AddDate(0, 0, -60), "inbound")
	e.seedInteraction(t, lapsed, now.AddDate(0, 0, -70), "inbound")
	set := []ids.ContactID{busy, lapsed, silent}
	touches := map[ids.ContactID]relstrength.Touches{busy: {}, lapsed: {}, silent: {}}

	ctx := e.as()
	statements := map[int]int{}
	together := map[ids.ContactID]relstrength.ChangeInputs{}
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
	if together[busy].Current.Count90d == together[lapsed].Current.Count90d {
		t.Fatalf("the busy and the lapsed contact fold alike (%+v), so a swapped row would pass", together[busy])
	}
	for _, contact := range set {
		if !reflect.DeepEqual(together[contact], alone[contact]) {
			t.Errorf("contact %s folds to %+v in a set and %+v alone", contact, together[contact], alone[contact])
		}
	}
}
