// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The second door an empty file can arrive at.
//
// The upload door refuses one outright, so the only way a row with no content
// reaches the tree is CAPTURE: an inbound message may legitimately carry an
// empty part, and recording what actually arrived is that path's job — it is
// not the connector's place to decide a received message was wrong. Sending one
// back out is a different question, and this is where it gets asked.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureAFile lands one attachment on the anchor through the REAL capture
// writer — staged then recorded, exactly as a connector does it. Seeding the
// row by hand would prove nothing about the column production writes.
func (e *sendEnv) captureAFile(t *testing.T, anchor ids.ActivityID, filename string, body []byte) ids.UUID {
	t.Helper()
	ctx := principal.WithCorrelationID(
		principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws),
			principal.Principal{
				Type: principal.PrincipalSystem, ID: "connector:imap",
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}},
				},
			}), ids.NewV7())
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).WithBlobstore(blobstore.NewMemory())
	staged, err := store.StageCapturedFiles(ctx, []CapturedFile{{
		PartID: "1", Filename: filename, ContentType: "application/pdf", Body: body,
	}})
	if err != nil {
		t.Fatalf("staging %q: %v", filename, err)
	}
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		return store.RecordCapturedFiles(ctx, tx, anchor, CapturedFileSource{
			System: "imap", MessageID: filename + "-" + anchor.String(),
			CapturedBy: "connector:imap", Category: "email_attachment",
		}, staged)
	}); err != nil {
		t.Fatalf("recording %q: %v", filename, err)
	}
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE entity_id = $1 AND filename = $2`, anchor, filename).Scan(&id); err != nil {
		t.Fatalf("reading back %q: %v", filename, err)
	}
	return id
}

// A file with no content cannot be sent, whichever channel would carry it.
//
// Left to the connector this parks the delivery under a transport error that
// reads like a transient condition, so the send burns its whole retry ladder on
// a file that will never have bytes — and every transport has to state the rule
// separately. Refused while staging, the sender is told once, in words naming
// the file.
//
// The captured file BESIDE it is not decoration: a refusal that fired on any
// attachment at all would satisfy the first half on its own.
func TestAnEmptyCapturedFileCannotBeSentBackOut(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")
	empty := e.captureAFile(t, anchor, "empty.pdf", []byte{})
	withBytes := e.captureAFile(t, anchor, "quote.pdf", []byte("%PDF-1.4"))

	// The row exists and records the truth — capture's job is to say what
	// arrived, so this is the state the send path has to be ready for.
	var size *int64
	if err := e.owner.QueryRow(context.Background(),
		`SELECT byte_size FROM attachment WHERE id = $1`, empty).Scan(&size); err != nil {
		t.Fatalf("reading the captured size: %v", err)
	}
	if size == nil || *size != 0 {
		t.Fatalf("the captured empty part recorded byte_size %v, want 0 — the fixture is not the shape under test", size)
	}

	withEmpty := soloSendInput("transactional")
	withEmpty.AttachmentIDs = []ids.UUID{empty}
	_, err := e.store(stubUnsubscribeLinker{}).SendEmail(
		e.as(principal.RowScopeAll), FromActivity(anchor), withEmpty, stubConsentGate{}, &recordingStager{})

	var emptyErr *EmptyAttachmentError
	if !errors.As(err, &emptyErr) {
		t.Fatalf("sending an empty file → %v, want EmptyAttachmentError", err)
	}
	if emptyErr.Filename != "empty.pdf" {
		t.Errorf("the refusal names %q, want the file the sender attached", emptyErr.Filename)
	}
	field, code, _ := emptyErr.FieldFault()
	if field != "attachment_ids" || code != "empty_attachment" {
		t.Errorf("field fault = (%q, %q), want (attachment_ids, empty_attachment)", field, code)
	}

	withReal := soloSendInput("transactional")
	withReal.AttachmentIDs = []ids.UUID{withBytes}
	if _, err := e.store(stubUnsubscribeLinker{}).SendEmail(
		e.as(principal.RowScopeAll), FromActivity(anchor), withReal, stubConsentGate{}, &recordingStager{}); err != nil {
		t.Fatalf("a file WITH content was refused too, so the rule is not about content: %v", err)
	}
}
