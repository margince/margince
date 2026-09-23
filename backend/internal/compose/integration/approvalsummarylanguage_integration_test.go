// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An approval card is written in the installation's base language.
//
// The census in compose proves every language HAS the sentence; only a real
// stager against a real settings row proves the right one is picked. This
// drives the held-send stager through the seam the send path calls, inside a
// workspace transaction, and reads the stored summary back.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdAndReadSummary stages one held message through the production notifier
// and answers the summary its card was stored with.
func holdAndReadSummary(ctx context.Context, t *testing.T, e *Env) string {
	t.Helper()
	notifier := compose.NewScheduledSendHeldNotifier(approvals.NewService(e.DB()))
	notice := activities.HeldNotice{
		ScheduledSendID: ids.NewV7(),
		ScheduledBy:     e.Rep1,
		Reason:          activities.HeldSendRefused,
		Subject:         "Angebot Q3",
		ScheduledAt:     time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
	var summary string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := notifier.NotifyHeldInTx(ctx, tx, notice); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT summary FROM approval
			 WHERE kind = $1 AND proposed_change->>'scheduled_send_id' = $2`,
			approvals.KindScheduledSendHeld, notice.ScheduledSendID.String()).Scan(&summary)
	}); err != nil {
		t.Fatalf("staging the held message's card: %v", err)
	}
	return summary
}

func TestAHeldSendCardIsWrittenInTheInstallationsBaseLanguage(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	setBaseLanguage(ctx, t, e.Pool, "de")
	german := `"Angebot Q3" wurde nicht gesendet: eine Prüfung hat den Versand zum Sendezeitpunkt abgelehnt`
	if got := holdAndReadSummary(ctx, t, e); got != german {
		t.Errorf("the installation is set to German and the card reads %q, want %q", got, german)
	}

	// The other direction catches a resolver that happens to return a
	// constant: a test asserting only German passes against code that always
	// writes German.
	setBaseLanguage(ctx, t, e.Pool, "en")
	english := `"Angebot Q3" was not sent: a gate refused it at send time`
	if got := holdAndReadSummary(ctx, t, e); got != english {
		t.Errorf("the installation was changed to English and the card reads %q, want %q", got, english)
	}
}
