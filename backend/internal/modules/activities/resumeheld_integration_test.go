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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type changingHeldMailAuthority struct {
	stubSendAuthority
	change func(context.Context) error
}

func (a changingHeldMailAuthority) SendableMailProvider(ctx context.Context) (string, error) {
	if err := a.change(ctx); err != nil {
		return "", err
	}
	return DefaultSendProvider, nil
}

func TestAResumeCannotSendWordingFromBeforeAConcurrentEdit(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true})
	anchor := e.seedAnchor(t, "", "")
	intent := store.holdForReview(ctx, FromActivity(anchor), soloSendInput("transactional"))
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
