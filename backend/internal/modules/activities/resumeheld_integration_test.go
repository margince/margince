// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/heldreason"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type changingHeldMailAuthority struct {
	stubSendAuthority
	change func(context.Context) error
}

func (a changingHeldMailAuthority) SendableMailbox(ctx context.Context, _ ids.UUID, _ ids.ActivityID) (string, ids.UUID, error) {
	if err := a.change(ctx); err != nil {
		return "", ids.Nil, err
	}
	return DefaultSendProvider, ids.Nil, nil
}

func TestAResumeCannotSendWordingFromBeforeAConcurrentEdit(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true})
	anchor := e.seedAnchor(t, "", "")
	intent := store.holdForReview(ctx, FromActivity(anchor), soloSendInput("transactional")).id
	if intent.IsZero() {
		t.Fatal("the refused message was not held")
	}
	store = store.WithSendAuthority(&changingHeldMailAuthority{
		change: func(ctx context.Context) error {
			_, err := e.owner.Exec(ctx, `UPDATE scheduled_send
				SET payload=jsonb_set(payload,'{body}','"Corrected wording."'::jsonb),version=version+1
				WHERE id=@intent`, pgx.StrictNamedArgs{"intent": intent})
			return err
		},
	})
	stager := &recordingStager{}
	if _, err := store.ResumeHeldSend(ctx, intent, stubConsentGate{}, stager); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("resuming an edited snapshot: %v", err)
	}
	if len(stager.staged) != 0 {
		t.Fatal("the stale prepared wording reached delivery")
	}
	var status, body string
	if err := e.owner.QueryRow(ctx, `SELECT status,payload->>'body' FROM scheduled_send WHERE id=@intent`,
		pgx.StrictNamedArgs{"intent": intent}).Scan(&status, &body); err != nil {
		t.Fatal(err)
	}
	if status != "held" || body != "Corrected wording." {
		t.Fatalf("message after the race: status=%s body=%q", status, body)
	}
}

// Only the seat that scheduled a held message resumes it, because the resume
// sends from the caller's mailbox. Another seat with the same grants is refused
// before the message is prepared, and the row stays held.
//
// Mutation: drop refuseAnotherSeatsHold from ResumeHeldSend and the other seat
// stages the delivery.
func TestOnlyTheAuthorResumesAHeldMessage(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true})
	intent := store.holdForReview(ctx, FromActivity(e.seedAnchor(t, "", "")), soloSendInput("transactional")).id
	if intent.IsZero() {
		t.Fatal("the refused message was not held")
	}
	author, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the rep's context carries no principal")
	}
	other := author
	other.UserID = ids.NewV7()
	other.ID = "human:" + other.UserID.String()
	colleague := principal.WithWorkspaceID(context.Background(), e.ws)
	colleague = principal.WithActor(principal.WithCorrelationID(colleague, ids.NewV7()), other)

	stager := &recordingStager{}
	var refused *heldreason.AuthorOnlyError
	if _, err := store.ResumeHeldSend(colleague, intent, stubConsentGate{}, stager); !errors.As(err, &refused) {
		t.Fatalf("another seat resuming the message: err = %v, want the author-only refusal", err)
	}
	if len(stager.staged) != 0 {
		t.Fatal("another seat's resume reached delivery")
	}
	var status string
	if err := e.owner.QueryRow(ctx, `SELECT status FROM scheduled_send WHERE id=@intent`,
		pgx.StrictNamedArgs{"intent": intent}).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "held" {
		t.Fatalf("the message reads %q after the refused resume, want held", status)
	}
}
