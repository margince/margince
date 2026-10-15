// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Hiding a moment is a PREFERENCE, not a correction.
//
// The distinction decides which store the write lands in, and getting it wrong
// leaks one contact's screen into everybody's data. A dismissal says "I have
// seen this and do not want to be told again"; it feeds no formula, no rule, no
// brief, no agent context and no other viewer's page (ADR-0096 D3). Saying a
// derived CLAIM is wrong is a different act with a different destination — the
// workspace-scoped AI feedback ledger, because a correction is shared truth.
//
// So this table carries no audit row and emits no event. There is no business
// fact here to reconstruct, and an outbox event announcing that somebody closed
// a card would be noise on a bus that carries record changes.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DismissContactMoment implements POST /contacts/{id}/moment/dismiss.
func (h Handlers) DismissContactMoment(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.DismissContactMomentRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	if body.ClaimKey == "" {
		httperr.Write(w, r, httperr.Validation("claim_key", "required",
			"name the moment to dismiss"))
		return
	}
	if body.EvidenceFingerprint == "" {
		// Without the fingerprint the dismissal could never re-arm, which makes
		// it a permanent silence nobody asked for.
		httperr.Write(w, r, httperr.Validation("evidence_fingerprint", "required",
			"pass the fingerprint the moment was showing, so the dismissal lifts when its evidence changes"))
		return
	}
	err := h.svc.DismissMoment(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)), body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreContactMoment implements DELETE /contacts/{id}/moment/dismiss.
func (h Handlers) RestoreContactMoment(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.RestoreContactMomentParams) {
	if params.ClaimKey == "" || params.EvidenceFingerprint == "" {
		httperr.Write(w, r, httperr.Validation("claim_key", "required",
			"name the moment to show again, with the fingerprint it was dismissed with"))
		return
	}
	err := h.svc.RestoreMoment(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)),
		crmcontracts.DismissContactMomentRequest(params))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DismissMoment records that this viewer has put this moment away while its
// evidence stands.
//
// Human-only, for the reason Acknowledge is: an agent principal carries the
// granting human's id, so resolving "the acting user" would let a passport read
// silence a card on a human's screen that the human never saw.
func (s *Service) DismissMoment(ctx context.Context, contactID ids.ContactID, in crmcontracts.DismissContactMomentRequest) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Anything that names a record is gated: dismissing a moment about a
		// contact the caller cannot read would confirm that contact exists.
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_moment_dismissal (user_id, contact_id, claim_key, evidence_fingerprint)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, contact_id, claim_key)
			DO UPDATE SET evidence_fingerprint = EXCLUDED.evidence_fingerprint,
			              dismissed_at = now()`,
			userID, contactID, in.ClaimKey, in.EvidenceFingerprint)
		if err != nil {
			// Wrapped: a bare pgx error carries the table and column it failed
			// on, and this one reaches a client through httperr.
			return fmt.Errorf("record the moment dismissal: %w", err)
		}
		return nil
	})
}

// RestoreMoment clears this viewer's dismissal of one moment: the undo behind
// "Not now". It asks the same gates the dismissal asks, so the readers who may
// set a moment aside are the ones who may undo it. It clears only the
// dismissal of this evidence, so an old toast's Undo leaves a newer one alone.
func (s *Service) RestoreMoment(ctx context.Context, contactID ids.ContactID, dismissed crmcontracts.DismissContactMomentRequest) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM contact_moment_dismissal
			WHERE user_id = $1 AND contact_id = $2 AND claim_key = $3 AND evidence_fingerprint = $4`,
			userID, contactID, dismissed.ClaimKey, dismissed.EvidenceFingerprint)
		if err != nil {
			return fmt.Errorf("clear the moment dismissal: %w", err)
		}
		return nil
	})
}
