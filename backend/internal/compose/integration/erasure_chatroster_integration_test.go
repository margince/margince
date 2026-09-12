// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Art. 17 over a chat roster: the third human in a group is named by an ACCOUNT
// and by nothing else, and an erasure that reached them only by address or by
// contact id would leave that account behind — readable, and matchable back to
// the subject by the next roster naming it.
//
// It also pins the pairing. An account id is a short opaque string, and the same
// one on a different transport is a different human; the scrub reads the
// transport off the activity the row hangs from, which is why the participant
// row carries none of its own.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The account the subject is on, and a decoy holding the SAME id on another
// transport — the over-deletion an untyped account match would commit.
const (
	subjectAccount   = "acct-selma-51"
	subjectTransport = "telegram"
	decoyTransport   = "whatsapp"
)

// seedRosterRowsFor puts the subject on two group chats: one on the transport
// their account belongs to, one on another transport that happens to issue the
// same id to somebody else. Returns the two participant row ids.
func seedRosterRowsFor(t *testing.T, e *Env, contactID ids.UUID) (mine, decoy ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, source, captured_by)
			VALUES ($1, $2, $3, 'capture', 'connector:telegram')`,
			contactID, subjectTransport, subjectAccount); err != nil {
			return err
		}
		for transport, into := range map[string]*ids.UUID{subjectTransport: &mine, decoyTransport: &decoy} {
			activityID := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, channel_provider)
				VALUES ($1, 'message', 'the group', now(), 'inbound', $2, 'connector:'||$2::text, $2)`,
				activityID, transport); err != nil {
				return err
			}
			// Named by ACCOUNT alone, which is what a chat roster gives: no
			// contact id, no address, nothing the other arms of the scrub see.
			row := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_participant (id, activity_id, channel_user_id, role)
				VALUES ($1, $2, $3, 'attendee')`, row, activityID, subjectAccount); err != nil {
				return err
			}
			*into = row
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return mine, decoy
}

func participantRowExists(t *testing.T, e *Env, row ids.UUID) bool {
	t.Helper()
	var found bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM activity_participant WHERE id = $1)`, row).Scan(&found)
	}); err != nil {
		t.Fatalf("reading the participant row back: %v", err)
	}
	return found
}

// The erasure reaches a party named only by their channel account, and stops at
// the transport boundary.
func TestErasingASubjectRemovesTheRosterRowsNamingTheirAccount(t *testing.T) {
	e := Setup(t)
	contactID := seedSubject(t, e)
	mine, decoy := seedRosterRowsFor(t, e, contactID)

	// The fixture has to start with both rows, or "gone" below is the state it
	// was seeded in rather than the state the erasure produced.
	if !participantRowExists(t, e, mine) || !participantRowExists(t, e, decoy) {
		t.Fatal("the fixture did not seed both roster rows")
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contactID, "art-17"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}

	if participantRowExists(t, e, mine) {
		t.Error("a roster row naming the erased subject by account survived — the account is still readable and still matchable back to them")
	}
	if !participantRowExists(t, e, decoy) {
		t.Error("a roster row on ANOTHER transport that merely reuses the account id was deleted — an account is only the subject's against the provider that issued it")
	}
}
