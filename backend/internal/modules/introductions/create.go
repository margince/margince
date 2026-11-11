// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fieldInternalReason = "internal_reason"

func (s *Store) createTx(ctx context.Context, tx pgx.Tx, id ids.UUID, actor principal.Principal, req NewRequest, capturedBy string) error {
	_, err := tx.Exec(ctx, `INSERT INTO intro_request (
 id, contact_id, requester_user_id, introducer_user_id, route_type, through_contact_id,
 internal_reason, value_for_target, forwardable_note, note_generated_by, note_ai_generated,
 fallback_policy, name_drop_allowed, due_at, captured_by)
 VALUES (
 @id, @contact_id, @requester_user_id, @introducer_user_id, @route_type, @through_contact_id,
 @internal_reason, @value_for_target, @forwardable_note, @note_generated_by, @note_ai_generated,
 @fallback_policy, @name_drop_allowed, @due_at, @captured_by)`, pgx.StrictNamedArgs{
		"id":                 id,
		fieldContactID:       req.ContactID,
		"requester_user_id":  actor.UserID,
		"introducer_user_id": req.IntroducerUser,
		"route_type":         req.RouteType,
		"through_contact_id": req.ThroughContactID,
		fieldInternalReason:  req.InternalReason,
		"value_for_target":   req.ValueForTarget,
		"forwardable_note":   req.ForwardableNote,
		"note_generated_by":  req.NoteGeneratedBy,
		"note_ai_generated":  req.NoteAIGenerated,
		"fallback_policy":    req.FallbackPolicy,
		"name_drop_allowed":  req.NameDropAllowed,
		"due_at":             req.DueAt,
		"captured_by":        capturedBy,
	})
	if err != nil {
		// The partial unique index arbitrates concurrent asks on the same route.
		if storekit.IsUniqueViolation(err) {
			return fmt.Errorf(
				"introductions: this colleague has already been asked about this contact: %w",
				apperrors.ErrConflict)
		}
		return fmt.Errorf("introductions: recording the ask: %w", err)
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "create", "intro_request", id,
		map[string]any{
			fieldContactID:        req.ContactID.String(),
			fieldIntroducerUserID: req.IntroducerUser.String(),
			"route_type":          req.RouteType,
		})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, req.ContactID,
		crmcontracts.PublicEventIntroRequestCreated{
			IntroRequestId:   openapi_types.UUID(id),
			ContactId:        openapi_types.UUID(req.ContactID),
			RequesterUserId:  openapi_types.UUID(actor.UserID),
			IntroducerUserId: openapi_types.UUID(req.IntroducerUser),
		})
}

func prepareAskRouteTx(ctx context.Context, tx pgx.Tx, req NewRequest) error {
	if err := auth.EnsureVisibleLive(ctx, tx, "contact", req.ContactID); err != nil {
		return err
	}
	// Naming an intermediary must not disclose a contact outside the reader’s scope.
	if req.ThroughContactID != nil {
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", *req.ThroughContactID); err != nil {
			return err
		}
	}
	if err := requireIntroducer(ctx, tx, req.IntroducerUser); err != nil {
		return err
	}
	return lockNamedContacts(ctx, tx, req.ContactID, req.ThroughContactID)
}
