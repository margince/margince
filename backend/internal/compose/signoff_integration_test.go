// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The sign-off a send appends, through the real wiring: the signature row
// contacts owns, the display name identity owns, and the composer's preview
// answering from the same code the send runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const signOffBody = "Hallo Anna, wie besprochen schicke ich dir das Angebot und die Unterlagen für das Projekt. Ich freue mich auf deine Rückmeldung."

// A sender with no signature signs off with a plain closing in the message's
// language and their name; once they write one, the send carries that
// signature and nothing else. Each time, the preview names the block the send
// then appended.
func TestASendSignsOffTheWayTheComposerPreviewSays(t *testing.T) {
	e := integration.Setup(t)
	anchorID, recipient := seedTransactionalReply(t, e)
	stager := &recordingStager{}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)

	send := func() string {
		t.Helper()
		if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{
			To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional",
		}); err != nil {
			t.Fatalf("send: %v", err)
		}
		return stager.staged[len(stager.staged)-1].Body
	}

	preview := previewSignOff(ctx, t, e)
	if preview.Kind != "closing" || preview.Text != "Viele Grüße,\nRep" {
		t.Fatalf("preview with no signature = %+v, want the German closing over the sender's name", preview)
	}
	if got, want := send(), signOffBody+"\n\n"+preview.Text; got != want {
		t.Fatalf("sent body = %q, want %q", got, want)
	}

	if _, err := contacts.NewStore(InstallationDB(e.Pool)).SaveMyEmailSignature(ctx, "Marek Janetzke\nGradion"); err != nil {
		t.Fatalf("save signature: %v", err)
	}
	preview = previewSignOff(ctx, t, e)
	if preview.Kind != "signature" || preview.Text != "Marek Janetzke\nGradion" {
		t.Fatalf("preview with a signature = %+v, want the signature alone", preview)
	}
	got := send()
	if want := signOffBody + "\n\n" + preview.Text; got != want {
		t.Fatalf("sent body = %q, want %q", got, want)
	}
	if strings.Contains(got, "Viele Grüße") {
		t.Fatalf("a signed send also carried the closing: %q", got)
	}
}

// previewSignOff asks POST /emails:sign-off through the handlers the server
// mounts.
func previewSignOff(ctx context.Context, t *testing.T, e *integration.Env) struct{ Text, Kind string } {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"body": signOffBody, "subject": "Angebot"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/emails:sign-off", bytes.NewReader(payload)).WithContext(ctx)
	rec := httptest.NewRecorder()
	newActivitiesHandlers(e.Pool).PreviewEmailSignOff(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview answered %d: %s", rec.Code, rec.Body.String())
	}
	var out struct{ Text, Kind string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	return out
}

// seedTransactionalReply seeds a contact who granted transactional mail and a
// thread to answer, so the send passes its gates and carries no footer.
func seedTransactionalReply(t *testing.T, e *integration.Env) (anchorID ids.UUID, recipient string) {
	t.Helper()
	admin := e.Admin()
	recipient = "anna@buyer.test"
	contact := e.SeedContact(t, "Anna Buyer", &e.Rep1)
	addContactEmail(t, e, contact, recipient)
	consentStore := consent.NewStore(InstallationDB(e.Pool))
	purpose, err := consentStore.CreatePurpose(admin, "transactional", "transactional", false)
	if err != nil {
		t.Fatalf("create purpose: %v", err)
	}
	if _, err := consentStore.Record(admin, consent.RecordInput{
		ContactID: ids.From[ids.ContactKind](contact), PurposeID: purpose.ID, NewState: "granted",
		PolicyText: &grantedWording,
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	anchorID = ids.NewV7()
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'Angebot', now(), 'manual', 'human:x')`, anchorID)
		return err
	}); err != nil {
		t.Fatalf("seed anchor: %v", err)
	}
	return anchorID, recipient
}
