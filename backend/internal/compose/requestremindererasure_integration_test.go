// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The reminder the real writer files quotes the sender in its body and its
// system text; erasing the mail's content takes the quote from both.
func TestErasingAMailTakesItsQuoteFromTheFiledReminder(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	var address string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&address); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, e.Rep1, address)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(reader, "address", "reply-alias@ourco.test"); err != nil {
		t.Fatal(err)
	}
	connector := principal.WithActor(reader, principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms})
	raw := []byte(strings.Join([]string{"From: Anna Weber <anna@customer.test>", "To: first@customer.test, reply-alias@ourco.test",
		"Subject: Product report", "Message-ID: <quoted-ask@customer.test>", "Date: " + at.Format(time.RFC1123Z),
		"Content-Type: text/plain; charset=utf-8", "", "Hello. Please send the report. Thanks."}, "\r\n"))
	parsed, err := mailmap.Parse(raw, "reply-alias@ourco.test")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(connector, parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	message := ref.ID
	contact := e.SeedContact(t, "Anna Weber", &e.AdminUser)
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](message), activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
		t.Fatal(err)
	}
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if _, err := e.Activities.SetOwedVerdictAsked(classifier, message, activities.OwedVerdictAsksUs, "fixture", at, "Please send the report."); err != nil {
		t.Fatal(err)
	}
	if err := NewOwedClassifier(e.Pool, nil, func() time.Time { return at.Add(time.Hour) }, nil).store.CaptureEmailRequests(classifier, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	const quoted = `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1
		AND (body LIKE '%Please send the report.%' OR system_text::text LIKE '%Please send the report.%')`
	if n := countIn(t, e, quoted+` AND body IS NOT NULL AND system_text::text LIKE '%Please send the report.%'`, message); n != 1 {
		t.Fatalf("%d reminder(s) quote the ask in body and system text, want 1; the test proves nothing otherwise", n)
	}

	eraser := eraserWithGraph(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB()))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return eraser.PurgeContentDerivedFromNoiseTx(e.Admin(), tx, []ids.UUID{message})
	}); err != nil {
		t.Fatal(err)
	}
	if n := countIn(t, e, quoted, message); n != 0 {
		t.Errorf("%d reminder(s) still quote the erased mail in body or system text", n)
	}
}
