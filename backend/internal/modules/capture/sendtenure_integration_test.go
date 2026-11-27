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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const day = 24 * time.Hour

// freshAddress is one no other test archived. The tables are shared across the
// workspaces a run seeds, and the bound reads every archive of an address.
func freshAddress() string { return "holder-" + ids.NewV7().String() + "@company.example" }

// seedAttestedSendTo records one provider-attested send to the address on a
// thread, at a given age.
func seedAttestedSendTo(ctx context.Context, tx pgx.Tx, address, thread string, ago time.Duration) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO activity (kind, occurred_at, direction, source, captured_by, thread_key,
		                      counterparty_email, counterparty_outbound_attested)
		VALUES ('email', now() - @age::interval, 'outbound', 'gmail', 'connector:gmail', @thread, @address, true)`,
		pgx.NamedArgs{"address": address, "thread": thread, "age": ago.String()})
	return err
}

// wroteToAfter seeds one send at sendAgo and an archive of the address at each
// of releasesAgo, then asks the send proof.
func wroteToAfter(t *testing.T, sendAgo time.Duration, releasesAgo ...time.Duration) bool {
	t.Helper()
	return wroteToGiven(t, sendAgo, func(ctx context.Context, tx pgx.Tx, address string) error {
		for _, ago := range releasesAgo {
			if err := archiveAddressAt(ctx, tx, address, ago); err != nil {
				return err
			}
		}
		return nil
	})
}

func wroteToGiven(t *testing.T, sendAgo time.Duration, history func(context.Context, pgx.Tx, string) error) bool {
	t.Helper()
	ctx, pool := meetingTenureEnv(t)
	address := freshAddress()
	var wrote bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := seedAttestedSendTo(ctx, tx, address, "thr-send", sendAgo); err != nil {
			return err
		}
		if err := history(ctx, tx, address); err != nil {
			return err
		}
		var err error
		wrote, err = partyWasWrittenToTx(ctx, tx, address)
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace wrote to the address: %v", err)
	}
	return wrote
}

func TestASendBeforeTheAddressLeftItsContactIsNotEvidence(t *testing.T) {
	t.Parallel()
	if wroteToAfter(t, 60*day, 30*day) {
		t.Error("a send from before the address was archived still counts, so the next " +
			"message mints the removed contact again")
	}
}

// The control: the bound ends old evidence, and new evidence still counts.
func TestASendAfterTheAddressLeftItsContactIsEvidence(t *testing.T) {
	t.Parallel()
	if !wroteToAfter(t, 30*day, 60*day) {
		t.Error("a send after the address was archived was refused, so the bound refuses " +
			"every send to an address that was ever archived")
	}
}

func TestTheNewestReleaseOfAnAddressIsItsBound(t *testing.T) {
	t.Parallel()
	if wroteToAfter(t, 45*day, 60*day, 30*day) {
		t.Error("a send between two archives of the address counts, so the bound reads " +
			"an older archive than the newest")
	}
}

// Restoring an archived contact clears archived_at on its addresses, which is
// what the unarchive writer leaves. The bound then falls back to the archive
// before it.
func TestAnUnarchiveRestoresTheEarlierBound(t *testing.T) {
	t.Parallel()
	wrote := wroteToGiven(t, 45*day, func(ctx context.Context, tx pgx.Tx, address string) error {
		if err := archiveAddressAt(ctx, tx, address, 60*day); err != nil {
			return err
		}
		if err := archiveAddressAt(ctx, tx, address, 30*day); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contact_email SET archived_at = NULL
			WHERE email = $1 AND archived_at > now() - interval '40 days'`, address)
		return err
	})
	if !wrote {
		t.Error("a send after the remaining archive was refused: the restored archive still bounds the address")
	}
}

// A meeting after the release creates the record. Sends from before it still
// read as an exchange, and they must not stand in for the meeting read.
func TestAMeetingAfterTheReleaseCreatesDespiteOlderSends(t *testing.T) {
	t.Parallel()
	ctx, pool := meetingTenureEnv(t)
	address := freshAddress()
	var dealt dealtWith
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		for _, thread := range []string{"thr-a", "thr-b"} {
			if err := seedAttestedSendTo(ctx, tx, address, thread, 60*day); err != nil {
				return err
			}
		}
		if err := archiveAddressAt(ctx, tx, address, 30*day); err != nil {
			return err
		}
		if err := seedMeetingWith(ctx, tx, address, 10*day); err != nil {
			return err
		}
		var err error
		dealt, err = (&Sink{}).dealtWithEnoughToRecord(ctx, tx,
			connector.Counterparty{Email: address, Domain: "company.example"}, ids.NewV7())
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace dealt with the address: %v", err)
	}
	if !dealt.corresponded || !dealt.exchanged {
		t.Fatalf("the older sends read corresponded=%v exchanged=%v, want both: the case is not set up",
			dealt.corresponded, dealt.exchanged)
	}
	if !dealt.create {
		t.Error("a meeting after the release did not create the record, because older sends skipped the meeting read")
	}
}
