// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const internalMailAuthorizationField = "internal_mail_authorization"

// MarkInternalMailTx retains the capture authorization and audience restriction across later imports and links.
func MarkInternalMailTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, authorization json.RawMessage) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if actor.Type != principal.PrincipalConnector || actor.UserID.IsZero() {
		return apperrors.ErrPermissionDenied
	}
	if err := auth.EnsureActivityVisible(ctx, tx, id.UUID); err != nil {
		return err
	}
	var existing json.RawMessage
	var audience string
	var reason *string
	var source, capturedBy string
	if err := tx.QueryRow(ctx, `SELECT internal_mail_authorization, audience, audience_reason, source, captured_by
  FROM activity WHERE id = $1 AND kind = 'email' AND restricted_at IS NULL
  AND archived_at IS NULL FOR UPDATE`, id).Scan(&existing, &audience, &reason, &source, &capturedBy); err != nil {
		return err
	}
	if len(existing) != 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity SET internal_mail_authorization = $2,
  audience = 'participants', audience_reason = $3 WHERE id = $1`, id, authorization, ReasonInternalOperational); err != nil {
		return err
	}
	if err := replaceAudienceMembers(ctx, tx, id, nil); err != nil {
		return err
	}
	if err := storekit.StampFields(ctx, tx, "activity", id.UUID, source, capturedBy,
		[]storekit.FieldStamp{{Field: internalMailAuthorizationField}}); err != nil {
		return err
	}
	digest := sha256.Sum256(authorization)
	before := map[string]any{auditFieldAudience: audience, auditFieldAudienceReason: reason, internalMailAuthorizationField: nil}
	after := map[string]any{auditFieldAudience: audienceParticipants, auditFieldAudienceReason: ReasonInternalOperational, internalMailAuthorizationField: hex.EncodeToString(digest[:])}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, before, after)
	if err != nil {
		return err
	}
	changed := crmcontracts.PublicEventActivityChangedFieldsAudience(audienceParticipants)
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &changed},
	})
}

func authorizeAudienceWriteTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in SetAudienceInput, members []AudienceMember, held bool) (bool, string, error) {
	var internalMail bool
	if err := tx.QueryRow(ctx, `SELECT internal_mail_authorization IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&internalMail); err != nil {
		return false, "", err
	}
	if !internalMail {
		return false, ReasonManual, auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
		return false, "", err
	}
	if err := auth.EnsureInternalMailParticipantTx(ctx, tx, id.UUID); err != nil {
		return false, "", err
	}
	if in.Audience == audienceWorkspace {
		return false, "", &InvalidAudienceError{Audience: in.Audience}
	}
	for _, member := range members {
		if member.SubjectType != string(crmcontracts.AudienceMemberSubjectTypeUser) {
			return false, "", apperrors.ErrInvalidArgument
		}
	}
	return true, ReasonInternalOperational, nil
}
