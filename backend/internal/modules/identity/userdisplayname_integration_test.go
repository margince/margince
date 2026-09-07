// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A member correcting the name their colleagues see them by. The properties
// worth a real database: the write is self-scoped, it carries the audit and
// outbox rows every mutation owes, re-saving the same name writes nothing, a
// blank name is refused, an agent acting under the member is refused, a
// deactivated seat is refused, and the roster reads the new name back.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// displayNameLedger counts the rows a name change is supposed to leave behind.
func displayNameLedger(t *testing.T, svc *Service, userID ids.UUID) (audits, events int) {
	t.Helper()
	ctx := context.Background()
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = 'user' AND entity_id = $1
			    AND after ? 'display_name'`, userID).Scan(&audits); err != nil {
			return err
		}
		// The type lives inside the envelope: the outbox row carries a stream
		// and a JSON envelope, not a typed column.
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox
			  WHERE envelope ->> 'type' = 'user_display_name.changed'`).
			Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

// storedName is what a colleague would see: read from `app_user` itself rather
// than from the answer the write returned, because every picker and the roster
// read it there.
func storedName(t *testing.T, svc *Service, userID ids.UUID) string {
	t.Helper()
	var name string
	ctx := context.Background()
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT display_name FROM app_user WHERE id = $1`, userID).Scan(&name)
	}); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestCorrectingYourNameWritesTheSeatItsAuditAndItsEvent(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	seat, err := svc.SaveMyDisplayName(ctx, "Ada Lovelace")
	if err != nil {
		t.Fatalf("correcting the name: %v", err)
	}
	if seat.DisplayName != "Ada Lovelace" {
		t.Errorf("the answer reports %q, want Ada Lovelace", seat.DisplayName)
	}
	if seat.UserID.UUID != userID {
		t.Errorf("the answer names user %s, want the caller %s", seat.UserID, userID)
	}
	// Through the column every other surface reads, not through the answer: a
	// write that returned the right name and stored the old one would pass an
	// assertion on `seat` alone.
	if got := storedName(t, svc, userID); got != "Ada Lovelace" {
		t.Errorf("app_user holds %q, want Ada Lovelace — the roster and every picker read it there", got)
	}

	audits, events := displayNameLedger(t, svc, userID)
	if audits != 1 {
		t.Errorf("audit rows naming a display name = %d, want 1 — a change to a seat is answerable later", audits)
	}
	if events != 1 {
		t.Errorf("user_display_name.changed events = %d, want 1 — the write shape owes an outbox row", events)
	}

	// Re-saving the SAME name is not a change. A settings form that saves on
	// every render would otherwise fill the ledger with a change nobody made.
	if _, err := svc.SaveMyDisplayName(ctx, "Ada Lovelace"); err != nil {
		t.Fatalf("re-asserting the same name: %v", err)
	}
	againAudits, againEvents := displayNameLedger(t, svc, userID)
	if againAudits != audits || againEvents != events {
		t.Errorf("re-saving the same name wrote %d audits / %d events, want the %d / %d already there",
			againAudits, againEvents, audits, events)
	}

	// And the trim is part of that sameness: the same name with spaces around
	// it is the same name, not a second change.
	if _, err := svc.SaveMyDisplayName(ctx, "  Ada Lovelace  "); err != nil {
		t.Fatalf("re-asserting the name with surrounding space: %v", err)
	}
	trimmedAudits, _ := displayNameLedger(t, svc, userID)
	if trimmedAudits != audits {
		t.Errorf("a name differing only by surrounding space wrote %d audits, want the %d already there",
			trimmedAudits, audits)
	}
}

func TestABlankOrOverlongNameIsRefused(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)
	before := storedName(t, svc, userID)

	// A refusal that names the CONTROL, not a sentence at the foot of the page:
	// the field is what points the settings form at the input the reader used.
	refusesNaming := func(t *testing.T, name string) {
		t.Helper()
		_, err := svc.SaveMyDisplayName(ctx, name)
		var fault apperrors.FieldFault
		if !errors.As(err, &fault) {
			t.Errorf("saving %q: err = %v, want a field fault", name, err)
			return
		}
		if field, _, _ := fault.FieldFault(); field != "display_name" {
			t.Errorf("saving %q faults on %q, want display_name", name, field)
		}
	}
	for _, name := range []string{"", "   ", "\t\n"} {
		refusesNaming(t, name)
	}
	// One rune over the contract's own bound, counted in RUNES: 255 bytes of a
	// multi-byte script is far fewer characters than the contract admits, and a
	// byte check would refuse a name the edge accepted.
	refusesNaming(t, strings.Repeat("é", 256))
	// Exactly at the bound is admitted — without this the case above would pass
	// against an implementation that refuses every multi-byte name.
	if _, err := svc.SaveMyDisplayName(ctx, strings.Repeat("é", 255)); err != nil {
		t.Errorf("saving exactly 255 characters: %v, want it admitted", err)
	}

	if got := storedName(t, svc, userID); got == before {
		t.Errorf("the 255-character name did not reach the column; it still holds %q", got)
	}
}

func TestAnAgentMayNotRenameItsGrantor(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)
	before := storedName(t, svc, userID)

	// An agent carrying this member's authority. It may act FOR them on their
	// records; it may not decide what they are called.
	agentCtx := principal.WithActor(ctx, principal.Principal{
		Type:   principal.PrincipalAgent,
		ID:     "agent:" + ids.NewV7().String(),
		UserID: userID,
	})
	if _, err := svc.SaveMyDisplayName(agentCtx, "Renamed By A Robot"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent renaming its grantor: err = %v, want ErrPermissionDenied", err)
	}
	if got := storedName(t, svc, userID); got != before {
		t.Errorf("the name became %q; a refused write must leave the column alone", got)
	}
}

func TestADeactivatedSeatMayNotRenameItself(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)
	before := storedName(t, svc, userID)

	// Deactivated, not archived — the state a departing colleague is left in,
	// and the one `archived_at IS NULL` alone would go on admitting.
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, userID)
		return err
	}); err != nil {
		t.Fatalf("deactivating the seat: %v", err)
	}

	if _, err := svc.SaveMyDisplayName(ctx, "Departed Colleague"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a deactivated seat renaming itself: err = %v, want ErrNotFound", err)
	}
	if got := storedName(t, svc, userID); got != before {
		t.Errorf("the name became %q; a departed colleague must not go on renaming themselves in the pickers their old records appear in", got)
	}
}

// The three rows of the write shape stand or fall together.
//
// A refused rename must leave NO audit row and NO event behind — a ledger entry
// for a change that never reached the domain row is worse than a missing one,
// because it is answerable later and wrong.
//
// What this case does NOT prove, said plainly: the FOR UPDATE on the read.
// Deactivating first means the READ refuses, so the update is never reached and
// the guard on its RowsAffected is not exercised — removing that guard leaves
// this green. The lock is what closes the window between the read and the
// write, and a serial test cannot open that window. It is held by review and by
// the comment at the query, not here.
func TestARefusedRenameWritesNoLedger(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)
	beforeAudits, beforeEvents := displayNameLedger(t, svc, userID)

	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, userID)
		return err
	}); err != nil {
		t.Fatalf("deactivating the seat: %v", err)
	}

	if _, err := svc.SaveMyDisplayName(ctx, "Never Landed"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("renaming a seat the write cannot reach: err = %v, want ErrNotFound", err)
	}
	audits, events := displayNameLedger(t, svc, userID)
	if audits != beforeAudits || events != beforeEvents {
		t.Errorf("a refused rename wrote %d audits / %d events, want the %d / %d already there",
			audits, events, beforeAudits, beforeEvents)
	}
}
