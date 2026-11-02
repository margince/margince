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
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storedSecret is the shape the shared rules recognise, written the way mail
// captured before the mask holds it.
const storedSecret = "Hunter2-Example!"

func runMailCredentialTask(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	engine, err := datatask.New(e.Pool, []datatask.Task{storedCredentialsMaskedDataTask()}, "test",
		func() time.Time { return clocktest.Now(t) },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// seedMailHoldingText writes a mail row as the capture path wrote one before
// it masked, so the repair has the shape it exists for.
func seedMailHoldingText(t *testing.T, e *integration.Env, subject, body string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, direction, source, captured_by, occurred_at)
		 VALUES ($1, 'email', $2, $3, 'inbound', 'manual', 'human:x', now() - interval '1 day')`,
		id, subject, body)
	return id
}

// A password stored before the mask is gone from the mail's own row, and the
// trail says the repair happened without repeating what it removed.
func TestTheRepairMasksACredentialAlreadyStoredInMail(t *testing.T) {
	e := integration.Setup(t)
	mail := seedMailHoldingText(t, e, "Zugang", "Passwort: "+storedSecret)
	clean := seedMailHoldingText(t, e, "Quarterly review", "Nothing secret here at all.")

	runMailCredentialTask(t, e)

	if body := e.WsScalar(t, `SELECT coalesce(body, '') FROM activity WHERE id = $1`, mail); strings.Contains(
		body, storedSecret) {
		t.Errorf("the stored password is still in the mail's body: %q", body)
	}
	// Read before the assertion below, so a repair that emptied every mail
	// could not pass as one that masked a credential.
	if after := e.WsScalar(t, `SELECT coalesce(body, '') FROM activity WHERE id = $1`, clean); after !=
		"Nothing secret here at all." {
		t.Errorf("a mail with no credential was rewritten: %q", after)
	}

	// The audit row records the repair and carries no images: the before-image
	// of these fields is the credential, and audit_log is append-only.
	images := e.WsScalar(t, `
		SELECT coalesce(string_agg(coalesce(before::text, '-') || ' ' || coalesce(after::text, '-'), ' '), '')
		  FROM audit_log WHERE entity_id = $1 AND entity_type = 'activity'`, mail)
	if strings.Contains(images, storedSecret) {
		t.Errorf("an audit image repeats the credential the repair removed: %q", images)
	}
	if !strings.Contains(images, "stored_credentials_masked") {
		t.Errorf("no audit row says the mail was repaired: %q", images)
	}
}
