// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrInternalMailHistoryPreviewChanged refuses consent against an older source policy.
var ErrInternalMailHistoryPreviewChanged = fmt.Errorf("preview the current internal-mail scope before starting: %w", apperrors.ErrConflict)

// InternalMailHistoryPreview binds an owner's separate consent to current administrator authorization.
type InternalMailHistoryPreview struct {
	Token         string
	ReceivedAfter time.Time
	Folders       []string
	admission     internalMailAdmission
}

type internalMailHistoryAuthorization struct {
	Admission    internalMailAdmission `json:"admission"`
	After        time.Time             `json:"after"`
	Before       time.Time             `json:"before"`
	AuthorizedBy string                `json:"authorized_by"`
}

type internalMailHistoryReceipt struct {
	BackfillID   ids.UUID  `json:"backfill_id"`
	After        time.Time `json:"after"`
	Before       time.Time `json:"before"`
	AuthorizedBy string    `json:"authorized_by"`
}

//nolint:nilnil // No offer means this owner has no authorized internal history to preview.
func internalMailHistoryPreviewTx(ctx context.Context, tx pgx.Tx, connection ids.UUID, owner ids.UserID, months int) (*InternalMailHistoryPreview, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() || actor.UserID != owner.UUID {
		return nil, nil
	}
	if err := settings.ShareLockForDecision(ctx, tx, internalMailPolicyKey); err != nil {
		return nil, err
	}
	policy, err := settings.ApplyTx(ctx, tx, internalMailPolicy)
	if err != nil || !policy.Enabled || policy.Since == nil {
		return nil, err
	}
	for _, source := range policy.Sources {
		if source.ConnectionID != connection || source.OwnerUserID != owner.UUID || source.HistorySince == nil {
			continue
		}
		current, err := internalMailSourceCurrentTx(ctx, tx, source)
		if err != nil || !current {
			return nil, err
		}
		admission := internalMailAdmission{InternalMailSource: source, PolicySince: *policy.Since}
		encoded, err := json.Marshal(struct {
			Admission internalMailAdmission
			Months    int
		}{admission, months})
		if err != nil {
			return nil, err
		}
		fingerprint := sha256.Sum256(encoded)
		return &InternalMailHistoryPreview{Token: hex.EncodeToString(fingerprint[:]), ReceivedAfter: *source.HistorySince, Folders: []string{}, admission: admission}, nil
	}
	return nil, nil
}

func sameInternalMailEnrollment(a, b InternalMailSource) bool {
	return sameInternalMailScope(a, b) && a.EnrolledAt.Equal(b.EnrolledAt) && a.EnrolledBy == b.EnrolledBy
}

func authorizeInternalMailHistoryTx(ctx context.Context, tx pgx.Tx, connection ids.UUID, owner ids.UserID, months int, token string, after time.Time) (*internalMailHistoryAuthorization, error) {
	preview, err := internalMailHistoryPreviewTx(ctx, tx, connection, owner, months)
	if err != nil {
		return nil, err
	}
	if preview == nil || preview.Token != token {
		return nil, ErrInternalMailHistoryPreviewChanged
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&now); err != nil {
		return nil, err
	}
	if preview.ReceivedAfter.After(after) {
		after = preview.ReceivedAfter
	}
	return &internalMailHistoryAuthorization{Admission: preview.admission, After: after, Before: now, AuthorizedBy: actor.ID}, nil
}

//nolint:nilnil // A run without current history authorization admits no internal mail.
func internalMailHistoryAdmissionTx(ctx context.Context, tx pgx.Tx, run ids.UUID, recReceived time.Time, source InternalMailSource, policySince time.Time) (*internalMailAdmission, error) {
	var encoded json.RawMessage
	err := tx.QueryRow(ctx, `SELECT internal_mail_authorization FROM capture_backfill
 WHERE id = $1 AND connection_id = $2 AND status IN ('queued','running')`, run, source.ConnectionID).Scan(&encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil || len(encoded) == 0 {
		return nil, err
	}
	var authorization internalMailHistoryAuthorization
	if err := json.Unmarshal(encoded, &authorization); err != nil {
		return nil, err
	}
	if !sameInternalMailEnrollment(authorization.Admission.InternalMailSource, source) ||
		!authorization.Admission.PolicySince.Equal(policySince) || recReceived.Before(authorization.After) || recReceived.After(authorization.Before) {
		return nil, nil
	}
	admitted := authorization.Admission
	admitted.History = &internalMailHistoryReceipt{BackfillID: run, After: authorization.After, Before: authorization.Before, AuthorizedBy: authorization.AuthorizedBy}
	return &admitted, nil
}

func (r *Registry) internalMailHistoryPreview(ctx context.Context, connection ids.UUID, owner ids.UserID, months int, after time.Time) (*InternalMailHistoryPreview, error) {
	var preview *InternalMailHistoryPreview
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		preview, err = internalMailHistoryPreviewTx(ctx, tx, connection, owner, months)
		return err
	})
	if err != nil || preview == nil {
		return nil, err
	}
	if after.After(preview.ReceivedAfter) {
		preview.ReceivedAfter = after
	}
	if len(preview.admission.Containers) == 0 {
		return preview, nil
	}
	folders, _, err := r.ListContainers(ctx, preview.admission.Provider, owner)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(folders))
	for _, folder := range folders {
		names[preview.admission.Provider+":"+folder.ID] = folder.Name
	}
	for _, selected := range preview.admission.Containers {
		name, found := names[selected]
		if !found {
			return nil, ErrInternalMailHistoryPreviewChanged
		}
		preview.Folders = append(preview.Folders, name)
	}
	return preview, nil
}
