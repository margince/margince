// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A send counts as the workspace writing to an address only from the moment the
// address last left a contact. That is the bound meetings already have, and it
// is what keeps a contact its owner removed from being minted again.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// seedAttestedSendTo records one provider-attested send to the address, at a
// given age.
func seedAttestedSendTo(ctx context.Context, tx pgx.Tx, address string, ago time.Duration) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO activity (kind, occurred_at, direction, source, captured_by,
		                      counterparty_email, counterparty_outbound_attested)
		VALUES ('email', now() - @age::interval, 'outbound', 'gmail', 'connector:gmail', @address, true)`,
		pgx.NamedArgs{"address": address, "age": ago.String()})
	return err
}

func wroteToAfter(t *testing.T, sendAgo, releaseAgo time.Duration) bool {
	t.Helper()
	ctx, pool := meetingTenureEnv(t)
	var wrote bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := seedAttestedSendTo(ctx, tx, recycledAddress, sendAgo); err != nil {
			return err
		}
		if err := archiveAddressAt(ctx, tx, recycledAddress, releaseAgo); err != nil {
			return err
		}
		var err error
		wrote, err = partyWasWrittenToTx(ctx, tx, recycledAddress)
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace wrote to the address: %v", err)
	}
	return wrote
}

func TestASendBeforeTheAddressLeftItsContactIsNotEvidence(t *testing.T) {
	if wroteToAfter(t, 60*24*time.Hour, 30*24*time.Hour) {
		t.Error("a send from before the address was archived still counts, so the next " +
			"message mints the removed contact again")
	}
}

// The control: the bound ends old evidence, and new evidence still counts.
func TestASendAfterTheAddressLeftItsContactIsEvidence(t *testing.T) {
	if !wroteToAfter(t, 30*24*time.Hour, 60*24*time.Hour) {
		t.Error("a send after the address was archived was refused, so the bound refuses " +
			"every send to an address that was ever archived")
	}
}
