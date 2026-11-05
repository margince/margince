// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A worklist notice the product raises for itself is stored copy, and the
// digest mail quotes its subject, so it answers to the installation.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheBacklogNoticeIsWrittenInTheInstallationsBaseLanguage(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	SetBaseLanguage(ctx, t, e.Pool, "de")
	subject, body := raiseBacklogNotice(ctx, t, e, e.Rep1, 4)
	if !strings.Contains(subject, "für dein Team noch nicht verfügbar") {
		t.Errorf("the installation is set to German and the notice reads %q", subject)
	}
	if !strings.Contains(body, "Absender mit ausstehender Prüfung: 4") {
		t.Errorf("the German body reads %q, and should count the waiting decisions", body)
	}

	// The other direction catches copy that happens to be one language: a test
	// asserting only German passes against code that always writes German.
	// A second seat, because the notifier dedupes per seat per day.
	//
	// Raising again for Rep1 would write nothing, and this would read the
	// German row back and pass.
	SetBaseLanguage(ctx, t, e.Pool, "en")
	subject, body = raiseBacklogNotice(ctx, t, e, e.Rep2, 7)
	if !strings.Contains(subject, "not yet available to your team") {
		t.Errorf("the installation was changed to English and the notice reads %q", subject)
	}
	if !strings.Contains(body, "Senders awaiting review: 7") {
		t.Errorf("the English body reads %q, and should count the waiting decisions", body)
	}
}

// raiseBacklogNotice raises the stall notice through the notifier the capture
// job wires, and answers the subject and body it stored.
//
// Through the real notifier rather than a notice written by hand. The language
// is resolved where the notifier builds the row, so a test writing its own row
// would prove nothing about the code under test.
func raiseBacklogNotice(
	ctx context.Context, t *testing.T, e *Env, seat ids.UUID, waiting int,
) (string, string) {
	t.Helper()
	store := notices.NewStore(compose.InstallationDB(e.Pool))
	raise := compose.NewBacklogStallNotifier(store, e.Pool, time.Now)
	if err := raise(ctx, seat, waiting); err != nil {
		t.Fatalf("raising the backlog notice: %v", err)
	}
	return readLatestNotice(ctx, t, e.Pool, seat)
}

// readLatestNotice reads the newest notice for one seat straight out of the
// table, because what this asserts is what was STORED.
func readLatestNotice(ctx context.Context, t *testing.T, pool *pgxpool.Pool, seat ids.UUID) (string, string) {
	t.Helper()
	var subject, body string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT subject, body FROM notice
			 WHERE recipient_user_id = $1
			 ORDER BY created_at DESC, id DESC
			 LIMIT 1`, seat).Scan(&subject, &body)
	}); err != nil {
		t.Fatalf("reading the stored notice: %v", err)
	}
	return subject, body
}
