// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reviewRecordingStager refuses every message at staging and records each
// review it is asked to open, with the held message the review would name.
type reviewRecordingStager struct {
	recordingStager
	intents []ids.UUID
}

func (r *reviewRecordingStager) RecordPendingReview(_ context.Context, err error, intent ids.UUID) error {
	r.intents = append(r.intents, intent)
	return err
}

// An erasure that won the race leaves nothing to hold and nothing to review: a
// review would store the erased address again. The sender still gets the refusal.
func TestARefusalForAnErasedRecipientOpensNoReview(t *testing.T) {
	t.Parallel()
	for _, erased := range []bool{false, true} {
		e := setupSend(t)
		ctx := e.as(principal.RowScopeAll)
		anchor := e.seedAnchor(t, "", "")
		if erased {
			if _, err := e.owner.Exec(context.Background(), `
				INSERT INTO erasure_suppression (kind, value_hash)
				VALUES ('email', encode(sha256(lower(trim($1))::bytea), 'hex'))
				ON CONFLICT DO NOTHING`, "buyer@example.test"); err != nil {
				t.Fatalf("recording the erasure: %v", err)
			}
		}
		stager := &reviewRecordingStager{err: apperrors.ErrConsentNotGranted}
		_, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), soloSendInput("transactional"), stubConsentGate{}, stager)
		if !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Fatalf("erased=%v: the sender got %v, want the consent refusal", erased, err)
		}
		var held int
		if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil {
			t.Fatal(err)
		}
		switch {
		case erased && (held != 0 || len(stager.intents) != 0):
			t.Errorf("an erased recipient's refusal held %d message(s) and asked for %d review(s), want none", held, len(stager.intents))
		case !erased && (held != 1 || len(stager.intents) != 1 || stager.intents[0].IsZero()):
			t.Errorf("a refusal held %d message(s) and asked for reviews %v, want one review naming the held message", held, stager.intents)
		}
	}
}
