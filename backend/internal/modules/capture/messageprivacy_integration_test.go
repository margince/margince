// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The capture decides once whether a message is private. A verdict that lands
// after that keeps the stored original, the files and the filing on the first
// answer, and only a record under another key, such as the seat-scoped copy of
// a collided message, asks again.
func TestAVerdictAfterThePrivacyDecisionLeavesEveryReaderOnTheFirstAnswer(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat := ids.NewV7()
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Seat')`,
			seat, seat.String()+"@seat.example"); err != nil {
			return err
		}
		ctx := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalConnector, ID: "connector:imap", UserID: seat})
		fields := ActivityFields{Kind: kindEmail, Direction: connector.DirectionInbound}
		rec := connector.NormalizedRecord{
			NaturalKey:   connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "privacy-" + seat.String()},
			Fields:       fields,
			ThreadKey:    "thread-" + seat.String(),
			Counterparty: connector.Counterparty{Email: "her@example.com", Direction: connector.DirectionInbound},
			Raw:          []byte("From: her@example.com\r\n\r\nBody."),
			Parts:        []connector.Part{{Ordinal: 1, Filename: "a.pdf", Body: []byte("%PDF")}},
		}
		decided, err := withMessagePrivacyTx(ctx, tx, rec, fields)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses)
			VALUES ($1, $2, 'held', 'personal', ARRAY['her@example.com'])`, rec.ThreadKey, seat); err != nil {
			return err
		}
		if private, _, err := capturedMessageIsPrivateTx(decided, tx, rec, fields); err != nil || private {
			t.Errorf("the files' reader answered private=%v (%v) after the decision said not private", private, err)
		}
		if private, err := capturedThreadIsPrivateTx(decided, tx, rec); err != nil || private {
			t.Errorf("the ladder answered private=%v (%v) after the decision said not private", private, err)
		}
		if kept, err := withholdOriginalPartsTx(decided, tx, rec); err != nil || len(kept.Parts) != 1 || string(kept.Raw) != string(rec.Raw) {
			t.Errorf("the stored original changed after the decision said not private (%v)", err)
		}
		scoped := rec
		scoped.NaturalKey.SourceID = connector.SeatScopedMailKey(rec.NaturalKey.SourceID, seat.String())
		if private, _, err := capturedMessageIsPrivateTx(decided, tx, scoped, fields); err != nil || !private {
			t.Errorf("the seat-scoped copy answered private=%v (%v); it must ask for itself", private, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
