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
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestTaskDue captures one request through the production mapper and
// confirms it as asking us. It runs the owed pass and returns the reminder's
// due moment.
func requestTaskDue(t *testing.T, e *integration.Env, subject string, sent time.Time) *time.Time {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1, 'gmail', 'shared', (SELECT email FROM app_user WHERE id=$1))`, e.Rep1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	receiver := scalar[string](t, e, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	raw := []byte(strings.Join([]string{
		"From: buyer@customer.test", "To: " + receiver,
		"Subject: " + subject, "Message-ID: <deadline-request@customer.test>",
		"Date: " + sent.Format(time.RFC1123Z), "Content-Type: text/plain; charset=utf-8", "",
		"Could you send it over? Thanks.",
	}, "\r\n"))
	message := captureThrough(t, e, "gmail", e.Rep1, receiver, raw)
	contact := e.SeedContact(t, "The buyer", &e.Rep1)
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](message), activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if _, err := e.Activities.SetCaptureLabel(ctx, message, "commitment"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.SetOwedVerdict(ctx, message, activities.OwedVerdictAsksUs, "prompts-test", sent); err != nil {
		t.Fatal(err)
	}
	classifier := compose.NewOwedClassifier(e.Pool, nil, func() time.Time { return sent.Add(time.Hour) }, slog.New(slog.DiscardHandler))
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatal(err)
	}
	return scalar[*time.Time](t, e, `SELECT due_at FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, message)
}

func installationDayEnd(t *testing.T, e *integration.Env, day string) time.Time {
	t.Helper()
	var end time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		name, err := identity.TimezoneAppliedTx(e.Admin(), tx)
		if err != nil {
			return err
		}
		zone, err := time.LoadLocation(name)
		if err != nil {
			return err
		}
		parsed, err := time.Parse(time.DateOnly, day)
		end = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, zone)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return end
}

func TestARequestThatStatesADeadlineIsDueAtTheEndOfThatDay(t *testing.T) {
	e := integration.Setup(t)
	sent := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	due := requestTaskDue(t, e, "Please send the logo | Deadline: 17 September", sent)
	want := installationDayEnd(t, e, "2026-09-17")
	if due == nil || !due.Equal(want) {
		t.Fatalf("due_at = %v, want the end of the stated day %v", due, want)
	}
}

func TestARequestWithNoOrAPastDeadlineStaysUndated(t *testing.T) {
	sent := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	for name, subject := range map[string]string{
		"no date":      "Please send the logo",
		"a past date":  "Please send the logo | Deadline: 2026-09-01",
		"an ambiguous": "Please send the logo | Deadline: 10/11/2026",
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			if due := requestTaskDue(t, e, subject, sent); due != nil {
				t.Fatalf("%s gave the reminder a due date: %v", name, due)
			}
		})
	}
}
