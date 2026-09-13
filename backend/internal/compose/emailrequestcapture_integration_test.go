// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCapturedRequestUsesTheParsedEnvelopeNotHeadersInsideTheBody(t *testing.T) {
	e := integration.Setup(t)
	// Connection configuration is the external boundary; messages and tasks
	// below land through the production mapper, sink and worker.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1, 'gmail', 'shared', (SELECT email FROM app_user WHERE id=$1)), ($2, 'imap', 'shared', (SELECT email FROM app_user WHERE id=$2))`, e.Rep1, e.Rep2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	receiver := scalar[string](t, e, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	copied := scalar[string](t, e, `SELECT email FROM app_user WHERE id = $1`, e.Rep2)
	at := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	raw := []byte(strings.Join([]string{
		"From: buyer@customer.test", "To: " + receiver, "Cc: " + copied,
		"Subject: Please send the report", "Message-ID: <requested-report@customer.test>",
		"Date: " + at.Format(time.RFC1123Z), "Content-Type: text/plain; charset=utf-8", "",
		"From: buyer@customer.test", "To: " + copied, "", "Please send the report.",
	}, "\r\n"))
	message := captureThrough(t, e, "gmail", e.Rep1, receiver, raw)
	if other := captureThrough(t, e, "imap", e.Rep2, copied, raw); other != message {
		t.Fatal("copies did not share their original")
	}
	contact := e.SeedContact(t, "The buyer", &e.Rep1)
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](message), activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if _, err := e.Activities.SetCaptureLabel(ctx, message, "commitment"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.SetOwedVerdict(ctx, message, activities.OwedVerdictAsksUs); err != nil {
		t.Fatal(err)
	}
	classifier := compose.NewOwedClassifier(e.Pool, nil, func() time.Time { return at.Add(time.Hour) }, slog.New(slog.DiscardHandler))
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatal(err)
	}

	assignee := scalar[ids.UUID](t, e, `SELECT assignee_id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, message)
	if assignee != e.Rep1 {
		t.Fatal("body text or CC delivery changed the directly addressed owner")
	}
	// The same production pass must be a replay, even without a model configured.
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatal(err)
	}
	count := scalar[int](t, e, `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, message)
	if count != 1 {
		t.Fatalf("capture replay created %d tasks", count)
	}
}
