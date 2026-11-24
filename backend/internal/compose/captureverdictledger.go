// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// verdictLedger is what judging one sender reads and settles through. It holds the
// owner's decision, the earlier answers about an address, the apply that resolves
// the row, and the retirement to a human. A certification run puts an in-memory one
// here, so the judgment it certifies is the engine's own.
type verdictLedger interface {
	OwnerDecided(ctx context.Context, row capture.PendingCounterparty) (decided bool, kind string, err error)
	TimesJudgedNotAContact(ctx context.Context, email string) (int, error)
	Settle(ctx context.Context, row capture.PendingCounterparty, kind string, ownerSaidSo bool, measured capture.VerdictMeasurement) (bool, error)
	Retire(ctx context.Context, row capture.PendingCounterparty, reason string, measured capture.VerdictMeasurement) error
}

// ledgerOf is the ledger this engine settles through. It is read each time and
// not set at build, because the engine is copied to give it a blobstore. A ledger
// bound to the first copy would apply with that copy's eraser.
//
//nolint:ireturn // the seam IS an interface: the database ledger in production, an in-memory one in a certification run.
func (e *CounterpartyVerdictEngine) ledgerOf() verdictLedger {
	if e.ledger != nil {
		return e.ledger
	}
	return databaseVerdictLedger{e}
}

// databaseVerdictLedger is the production ledger: the pending store and the
// transactions the engine applies a verdict in.
type databaseVerdictLedger struct{ e *CounterpartyVerdictEngine }

func (l databaseVerdictLedger) OwnerDecided(ctx context.Context, row capture.PendingCounterparty) (bool, string, error) {
	return l.e.ownerDecided(ctx, row)
}

func (l databaseVerdictLedger) TimesJudgedNotAContact(ctx context.Context, email string) (int, error) {
	return l.e.pending.TimesJudgedNotAContact(ctx, email)
}

func (l databaseVerdictLedger) Settle(
	ctx context.Context, row capture.PendingCounterparty, kind string, ownerSaidSo bool, measured capture.VerdictMeasurement,
) (bool, error) {
	return l.e.apply(ctx, row, kind, ownerSaidSo, measured)
}

func (l databaseVerdictLedger) Retire(
	ctx context.Context, row capture.PendingCounterparty, reason string, measured capture.VerdictMeasurement,
) error {
	return l.e.pending.Retire(ctx, row, reason, measured)
}

// unusableVerdictAnswer is a model reply the engine could not read: it did not
// parse, or it broke the id contract. It is told apart from a failed call so a
// certification run can count it against the model.
type unusableVerdictAnswer struct{ err error }

func (e *unusableVerdictAnswer) Error() string { return e.err.Error() }
func (e *unusableVerdictAnswer) Unwrap() error { return e.err }
