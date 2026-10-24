// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// audit_log is append-only, so a password an edit writes into an image stays in
// the history for good. These tests read the stored images and the row.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	maskablePlain = "Hunter2-Example!"
	maskableLine  = "Passwort: " + maskablePlain
	maskedLine    = "Passwort: " + credentialmask.Marker
	callNotes     = "Call notes"
)

// auditHoldsPlain reports whether any audit row, of any record type, holds the
// password in clear in its before, after or evidence.
func auditHoldsPlain(t *testing.T, e *sendEnv) bool {
	t.Helper()
	var holds bool
	if err := e.owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM audit_log
		  WHERE concat_ws(' ', before::text, after::text, evidence::text) LIKE '%' || $1 || '%')`,
		maskablePlain).Scan(&holds); err != nil {
		t.Fatalf("reading the audit rows: %v", err)
	}
	return holds
}

func storedSubjectAndBody(t *testing.T, e *sendEnv, activityID ids.UUID) (subject, body string) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(subject, ''), coalesce(body, '') FROM activity WHERE id = $1`, activityID,
	).Scan(&subject, &body); err != nil {
		t.Fatalf("reading the activity: %v", err)
	}
	return subject, body
}

func loggedActivity(ctx context.Context, t *testing.T, e *sendEnv, kind, body string) ids.UUID {
	t.Helper()
	subject := noteSubject
	request := crmcontracts.CreateActivityRequest{Kind: crmcontracts.CreateActivityRequestKind(kind), Subject: &subject, Body: &body, Source: "manual"}
	if kind == string(crmcontracts.ActivityKindEmail) || kind == string(crmcontracts.ActivityKindMessage) {
		direction := crmcontracts.CreateActivityRequestDirectionInbound
		request.Direction = &direction
	}
	if kind == string(crmcontracts.ActivityKindMessage) {
		provider := "telegram"
		request.ChannelProvider = &provider
	}
	in, err := LogActivityInputFrom(request)
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return ids.UUID(activity.Id)
}

func editBody(ctx context.Context, t *testing.T, e *sendEnv, id ids.UUID, body string) {
	t.Helper()
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](id), UpdateActivityInput{Body: &body}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
}

func TestAnEditThatRemovesAPasswordRecordsTheReplacedTextMasked(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	id := loggedActivity(ctx, t, e, "note", callNotes+"\n"+maskableLine)

	editBody(ctx, t, e, id, callNotes)

	before, after := auditImagesFor(t, e, id)
	if want := callNotes + "\n" + maskedLine; before["body"] != want {
		t.Errorf("before[body] = %q, want %q", before["body"], want)
	}
	if after["body"] != callNotes {
		t.Errorf("after[body] = %q, want %q", after["body"], callNotes)
	}
	if auditHoldsPlain(t, e) {
		t.Error("an audit row of the note still holds the password in clear")
	}
}

func TestANoteKeepsThePasswordItsMemberTypedAndItsAuditDoesNot(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	id := loggedActivity(ctx, t, e, "note", callNotes)

	editBody(ctx, t, e, id, callNotes+"\n"+maskableLine)

	if _, body := storedSubjectAndBody(t, e, id); body != callNotes+"\n"+maskableLine {
		t.Errorf("the note's body = %q, want the text as the member typed it", body)
	}
	_, after := auditImagesFor(t, e, id)
	if want := callNotes + "\n" + maskedLine; after["body"] != want {
		t.Errorf("after[body] = %q, want %q", after["body"], want)
	}
	if auditHoldsPlain(t, e) {
		t.Error("an audit row of the note holds the password in clear")
	}
}

func TestAnEditToAMailStoresItsPasswordMasked(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	id := loggedActivity(ctx, t, e, "email", callNotes)

	subject, body := maskableLine, callNotes+"\n"+maskableLine
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](id),
		UpdateActivityInput{Subject: &subject, Body: &body}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	storedSubject, storedBody := storedSubjectAndBody(t, e, id)
	if storedSubject != maskedLine {
		t.Errorf("the mail's subject = %q, want %q", storedSubject, maskedLine)
	}
	if want := callNotes + "\n" + maskedLine; storedBody != want {
		t.Errorf("the mail's body = %q, want %q", storedBody, want)
	}
	if auditHoldsPlain(t, e) {
		t.Error("an audit row of the mail holds the password in clear")
	}
}

// A mail edit that only resends the masked text the row holds changes nothing,
// so it writes no audit row.
func TestResendingAMailsPasswordChangesNothing(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	id := loggedActivity(ctx, t, e, "email", callNotes)
	editBody(ctx, t, e, id, maskableLine)

	resent := maskableLine
	updated, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](id), UpdateActivityInput{Body: &resent})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	var updates int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`, id,
	).Scan(&updates); err != nil {
		t.Fatalf("counting the updates: %v", err)
	}
	if updates != 1 {
		t.Errorf("%d update rows, want 1: the second edit sent what the row already holds", updates)
	}
	if updated.Body == nil || *updated.Body != maskedLine {
		t.Errorf("the second edit answered a body other than %q", maskedLine)
	}
}

// Every writer of these record types reaches audit_log through storekit, so
// the row is written here through it directly, with the text in every column.
func TestEveryColumnOfAMailTextAuditRowIsStoredMasked(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	for _, entityType := range []string{"activity", "scheduled_send", "mail_draft"} {
		image := map[string]any{"subject": maskableLine}
		err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			_, err := storekit.AuditWithEvidence(ctx, tx, "update", entityType, ids.NewV7(), image, image,
				map[string]any{"reason": maskableLine})
			return err
		})
		if err != nil {
			t.Fatalf("auditing a %s: %v", entityType, err)
		}
	}
	if auditHoldsPlain(t, e) {
		t.Error("an audit row holds the password in clear")
	}
}

// A mail or a channel message logged by hand or imported is stored masked, as
// capture stores it. A later edit masks the same way. A note is not masked.
func TestALoggedMailOrMessageStoresItsPasswordMasked(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	for kind, want := range map[string]string{
		"email":   callNotes + "\n" + maskedLine,
		"message": callNotes + "\n" + maskedLine,
		"note":    callNotes + "\n" + maskableLine,
	} {
		id := loggedActivity(ctx, t, e, kind, callNotes+"\n"+maskableLine)
		if _, body := storedSubjectAndBody(t, e, id); body != want {
			t.Errorf("a logged %s stored body %q, want %q", kind, body, want)
		}
	}
	message := loggedActivity(ctx, t, e, "message", callNotes)
	editBody(ctx, t, e, message, maskableLine)
	if _, body := storedSubjectAndBody(t, e, message); body != maskedLine {
		t.Errorf("an edited message stored body %q, want %q", body, maskedLine)
	}
}
