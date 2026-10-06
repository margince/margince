// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The eraser still refuses the tombstone address, for rows written before the parser
// began refusing it.
//
// values.ParseEmail rejects values.ErasedEmail, so no new contact can hold it — but an
// installation migrated from before that can, and a seat is resolved by ADDRESS. The
// guard answers from the address list alone and returns before any statement runs, so
// this needs no database: a nil transaction proves it never reaches one.
func TestEraseDealRoomSeatsRefusesTheTombstoneAddress(t *testing.T) {
	t.Parallel()
	err := eraseDealRoomSeats(context.Background(), nil,
		[]string{"kept@example.com", values.ErasedEmail}, "subject request")
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("eraseDealRoomSeats → %v, want a conflict: an address that names every seat an "+
			"earlier erasure wiped cannot be told from this subject's own", err)
	}
}
