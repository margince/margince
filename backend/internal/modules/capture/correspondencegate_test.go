// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// An address that folds to nothing is not correspondence, and every gate here
// says so BEFORE it queries.
//
// The guard is what lets a caller pass whatever a provider put in the header.
// Without it a message with a blank or whitespace From would ask the database
// about the empty address — and `counterparty_email = ''` is a value rows can
// genuinely hold, so the answer would be some other message's, attributed to a
// sender nobody named.
//
// A nil transaction is the assertion: each of these must return before it
// reaches one, so a test that gets an answer rather than a panic has proved the
// guard is there.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNoGateAsksTheDatabaseAboutAnEmptyAddress(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, address := range []string{"", "   "} {
		t.Run("address "+address, func(t *testing.T) {
			t.Parallel()

			if got, err := correspondencePositiveTx(ctx, nil, address); got || err != nil {
				t.Errorf("the correspondence gate answered (%v, %v) for a blank address", got, err)
			}
			if got, err := wroteBackTx(ctx, nil, address); got || err != nil {
				t.Errorf("the wrote-back gate answered (%v, %v) for a blank address", got, err)
			}
			if got, err := wroteOnTwoThreadsTx(ctx, nil, address); got || err != nil {
				t.Errorf("the two-threads gate answered (%v, %v) for a blank address", got, err)
			}
			met, elsewhere, err := metInPersonTx(ctx, nil, address, ids.NewV7())
			if met || elsewhere || err != nil {
				t.Errorf("the met-in-person gate answered (%v, %v, %v) for a blank address",
					met, elsewhere, err)
			}
		})
	}
}
