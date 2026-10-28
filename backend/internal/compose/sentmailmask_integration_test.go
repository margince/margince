// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The sent row and the sender's synced copy of one mail must read alike.
// Otherwise the echo that joins the row would have to rewrite it.
func TestASentMailKeepsOneMaskedRowThroughItsEcho(t *testing.T) {
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	root := "masked-opening@counterparty.test"
	captureThreadMail(t, e, e.Rep1, threadMail(address, root, "Tue, 01 Sep 2026 10:00:00 +0000", ""))
	var anchor ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT activity_id FROM activity_identity WHERE identity_kind = 'mail' AND identity_key = $1`, root).Scan(&anchor)
	}); err != nil {
		t.Fatal(err)
	}
	const secret = "Hunter2-Example!"
	stager := &recordingStager{}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	sent, err := adapter.SendEmail(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), anchor, agents.SendEmailArgs{
		To: []string{"pat@counterparty.example"}, Subject: "Zugang Passwort: " + secret,
		Body: "Hallo Pat,\nPasswort: " + secret + "\nBis bald", ConsentPurpose: "transactional",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStaged(t, stager, 1, "the send")
	delivered := stager.staged[0]
	if !strings.Contains(delivered.Subject, secret) || !strings.Contains(delivered.Body, secret) {
		t.Fatalf("the delivery carries %q / %q, want the text as typed", delivered.Subject, delivered.Body)
	}

	stored := func() (messageID, subject, body string) {
		t.Helper()
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT source_id, subject, body FROM activity WHERE id = $1`, sent.ActivityID).Scan(&messageID, &subject, &body)
		}); err != nil {
			t.Fatal(err)
		}
		return messageID, subject, body
	}
	messageID, subject, body := stored()
	if subject != credentialmask.MaskText(delivered.Subject) || body != credentialmask.MaskText(delivered.Body) {
		t.Fatalf("the sent row stores %q / %q, want the delivered text masked", subject, body)
	}

	echo := []byte(strings.Join([]string{
		"From: " + address,
		"To: pat@counterparty.example",
		"Subject: " + delivered.Subject,
		"Date: Sun, 18 Oct 2026 10:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"References: <" + root + ">",
		"Content-Type: text/plain; charset=utf-8", "", delivered.Body, "",
	}, "\r\n"))
	assertCaptureMasksAsTheSendDid(t, echo, address, subject, body)
	// Every row, not the rows under the send's key: a copy the replay proof
	// refuses is filed under a key of its own.
	before := e.WsCount(t, `SELECT count(*) FROM activity`)
	captureThreadMail(t, e, e.Rep1, echo)
	captureThreadMail(t, e, e.Rep1, echo)
	if added := e.WsCount(t, `SELECT count(*) FROM activity`) - before; added != 0 {
		t.Fatalf("the echo added %d rows, want it to join the sent row", added)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, sent.ActivityID); count != 1 {
		t.Fatalf("the echo left %d import rows on the sent row, want 1", count)
	}
	if _, afterSubject, afterBody := stored(); afterSubject != subject || afterBody != body {
		t.Fatalf("the echo rewrote the sent row to %q / %q", afterSubject, afterBody)
	}
}

// assertCaptureMasksAsTheSendDid holds capture's stored text for the echo to
// the sent row's: the same subject, and the same body under capture's
// From/To lines.
func assertCaptureMasksAsTheSendDid(t *testing.T, raw []byte, mailbox, subject, body string) {
	t.Helper()
	parsed, err := mailmap.Parse(raw, mailbox)
	if err != nil {
		t.Fatalf("parsing the echo: %v", err)
	}
	fields, ok := parsed.ToRecord("gmail", raw).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("the echo maps to no activity")
	}
	if fields.Subject != subject || !strings.HasSuffix(fields.Body, "\n\n"+body) {
		t.Fatalf("capture would store %q / %q for the echo, want %q / ...%q", fields.Subject, fields.Body, subject, body)
	}
}

// The installation's own mail is a sent row too, so its timeline copy is masked
// while the delivery keeps the words it sends.
func TestControllerMailStoresItsTimelineRowMasked(t *testing.T) {
	l := setupConfirmLane(t)
	inserter, err := jobs.NewInserter(l.e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	// The live link is the engine's evidence for a record confirmation.
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	var link ids.UUID
	if err := l.e.Pool.QueryRow(l.ctx, `SELECT id FROM confirm_token WHERE contact_id = $1`, l.contact).Scan(&link); err != nil {
		t.Fatalf("reading the confirm link: %v", err)
	}
	expires := clocktest.Now(t).Add(time.Hour)
	rendered, category, err := consent.RenderControllerTemplate(consent.TemplateRecordConfirmation, expires, "")
	if err != nil {
		t.Fatalf("rendering the template: %v", err)
	}
	const secret = "Hunter2-Example!"
	rendered.Subject += " Passwort: " + secret
	rendered.Body += "\nPasswort: " + secret
	var deliveredSubject, deliveredBody, storedSubject, storedBody string
	if err := database.WithWorkspaceTx(l.ctx, l.e.Pool, func(tx pgx.Tx) error {
		delivery, err := NewControllerMailQueue(l.e.Pool, inserter, "").QueueConfirmationTx(l.ctx, tx, consent.ConfirmationSend{
			ContactID: ids.From[ids.ContactKind](l.contact), Recipient: "subject@example.test", Category: category,
			LinkID: link, LinkRef: "sealed-ref", ExpiresAt: expires,
			MessageID: "controller-mask@margince.test", Rendered: rendered,
		})
		if err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `
			SELECT o.subject, o.body, a.subject, a.body
			  FROM comms_outbound o JOIN activity a ON a.id = o.activity_id
			 WHERE o.id = $1`, delivery).Scan(&deliveredSubject, &deliveredBody, &storedSubject, &storedBody)
	}); err != nil {
		t.Fatalf("queueing the controller mail: %v", err)
	}
	if !strings.Contains(deliveredSubject, secret) || !strings.Contains(deliveredBody, secret) {
		t.Fatalf("the delivery carries %q / %q, want the words as rendered", deliveredSubject, deliveredBody)
	}
	readable := strings.ReplaceAll(deliveredBody, "{{confirmation-link}}", mailcopy.For(rendered.Language).SentLinkMarker)
	if storedSubject != credentialmask.MaskText(deliveredSubject) || storedBody != credentialmask.MaskText(readable) {
		t.Fatalf("the timeline row stores %q / %q, want the delivered text masked", storedSubject, storedBody)
	}
}
