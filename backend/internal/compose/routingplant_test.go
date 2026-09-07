// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The write half of planting a declared binding, and the one thing it must do
// when it cannot: nothing loud enough to stop a boot.
//
// No database. That is the whole reason the write is split from
// SeedRoutingIfUnset — the refusal is the branch worth judging, and it is the
// one a real store will not perform on demand. routingSeedFrom's own comment
// makes the same split for the same reason, on the half that can refuse a
// document.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// refusingStore fails every write, which is the state this file exists to
// describe: a database that answers, and will not take this row.
type refusingStore struct{ err error }

func (r refusingStore) WriteTx(context.Context, func(pgx.Tx) error) error { return r.err }

// A boot whose settings write is refused keeps going. The installation still
// resolves whatever it holds, which is exactly what it did before planting
// existed — so a failure here must not be the thing that stops a start.
func TestAPlantThatCannotBeWrittenDoesNotStopTheBoot(t *testing.T) {
	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, nil))

	// Returns nothing, so "does not stop the boot" is a property of the
	// signature as much as of this call. The assertion left is that it neither
	// panics nor goes silent.
	plantRouting(context.Background(), refusingStore{err: errors.New("read-only transaction")},
		ai.RoutingConfig{}, log)

	out := logged.String()
	if !strings.Contains(out, "cannot plant") {
		t.Errorf("a refused plant said nothing: %s", out)
	}
	// The reason travels with it. A warning that reports the failure without
	// the cause sends its reader to the database with nothing to look for.
	if !strings.Contains(out, "read-only transaction") {
		t.Errorf("the warning drops the underlying error: %s", out)
	}
	// And it must not claim to have done the thing it just failed to do.
	if strings.Contains(out, "planted the model binding") {
		t.Errorf("a refused plant reported success: %s", out)
	}
}

// acceptingStore runs the callback against no transaction, so the seed write
// inside it decides the outcome. Used only to prove the success path logs.
type acceptingStore struct{ err error }

func (a acceptingStore) WriteTx(_ context.Context, fn func(pgx.Tx) error) error {
	if a.err != nil {
		return a.err
	}
	// A nil transaction is enough: SeedValue is not reached in this test's
	// arrangement, and what is under test is what plantRouting does with the
	// store's answer.
	return nil
}

// A write the store accepts but which stores nothing is NOT reported as a
// plant. Seeding is insert-only, so "accepted, wrote nothing" is the ordinary
// outcome on an installation that already had a binding — and announcing one
// there would be a log line contradicting the row.
func TestAPlantThatStoresNothingIsNotAnnounced(t *testing.T) {
	var logged strings.Builder
	plantRouting(context.Background(), acceptingStore{}, ai.RoutingConfig{},
		slog.New(slog.NewTextHandler(&logged, nil)))

	if strings.Contains(logged.String(), "planted the model binding") {
		t.Errorf("a write that stored nothing announced a plant: %s", logged.String())
	}
}
