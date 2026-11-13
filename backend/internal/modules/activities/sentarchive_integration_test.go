// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// What a member sent is the record of what a customer received, so its sender
// archives it only with activity.delete, like captured correspondence.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// onAFullSeat is ctx's caller on a full seat, which the archivable flag asks
// for before any grant.
func onAFullSeat(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no caller to seat")
	}
	p.SeatType = principal.SeatFull
	return principal.WithActor(ctx, p)
}

// assertTheSenderCannotArchive reads the row as its sender, who holds update
// without delete, and archives it as them.
func assertTheSenderCannotArchive(t *testing.T, e *sendEnv, store *Store, id ids.ActivityID, what string) {
	t.Helper()
	sender := onAFullSeat(e.as(principal.RowScopeAll), t)
	read, err := store.GetActivity(sender, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the %s back: %v", what, err)
	}
	if read.Archivable == nil || *read.Archivable {
		t.Errorf("the sender reads the %s as archivable, want not archivable", what)
	}
	if _, err := store.ArchiveActivity(sender, id, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the sender archiving the %s → %v, want ErrPermissionDenied", what, err)
	}
}

func TestASenderCannotArchiveAnEmailTheySentBeforeOrAfterItsEchoMerges(t *testing.T) {
	e := setupSend(t)
	store := e.store(stubUnsubscribeLinker{})
	sent, err := store.SendEmail(e.as(principal.RowScopeAll), FromActivity(e.seedAnchor(t, "", "")),
		sendInput("transactional"), stubConsentGate{}, &recordingStager{})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(sent.Id))
	assertTheSenderCannotArchive(t, e, store, id, "sent email")

	echo := ids.New[ids.ActivityKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source_system, source_id, source, captured_by, thread_key, counterparty_email)
		VALUES ($1, 'email', $2, $3, now(), 'outbound', $4, $5, $6, 'connector:gmail', $5, $7)`,
		echo, capturedEchoSubject, capturedEchoBody, connector.EmailSourceSystem, stampedIdentity, "gmail:"+stampedIdentity, counterparty); err != nil {
		t.Fatalf("seeding the provider's copy of the sent email: %v", err)
	}
	worker := connector.WithSentEcho(e.asSendWorker(), connector.SentEcho{
		Identity: stampedIdentity, Subject: capturedEchoSubject, Body: capturedEchoBody,
	})
	if err := database.WithWorkspaceTx(worker, e.pool, func(tx pgx.Tx) error {
		return store.ReconcileMessageIdentityTx(worker, tx, id, *sent.SourceId, stampedIdentity)
	}); err != nil {
		t.Fatalf("ReconcileMessageIdentityTx: %v", err)
	}
	if !e.absorbedRow(t, echo).archived || e.sentRow(t, id).sourceID != stampedIdentity {
		t.Fatal("the provider's copy did not merge onto the sent email")
	}
	assertTheSenderCannotArchive(t, e, store, id, "sent email after its echo merged")
}

func TestASenderCannotArchiveAChannelMessageTheySent(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedChannelAnchor(t)
	contact := e.linkContact(t, anchor, "Telegram Buyer")
	store := e.channelStore(reaches(map[ids.UUID]string{contact: testChannelAccount}))
	sent, err := store.SendMessage(e.as(principal.RowScopeAll), anchor, channelInput(), &recordingConsentGate{}, &recordingChannelStager{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	assertTheSenderCannotArchive(t, e, store, ids.From[ids.ActivityKind](ids.UUID(sent.Id)), "channel message")
}
