// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ReuseWebsiteArrivalTask enriches the immediate contact-owner task without reopening work.
func (s *Store) ReuseWebsiteArrivalTask(ctx, admit context.Context, key string, source, recipient ids.UUID, due time.Time) (bool, error) {
	if err := auth.Require(admit, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	if err := auth.Require(admit, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	found := false
	run := func() error {
		found = false
		return database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(bound context.Context) error {
			return s.tx(bound, func(tx pgx.Tx) error {
				system, sourceKey := provenance.WebsiteInquiryTaskSource(key, true)
				var id ids.ActivityID
				err := tx.QueryRow(bound, `SELECT id FROM activity WHERE source_system=@system AND source_id=@key FOR UPDATE`, pgx.NamedArgs{"system": system, "key": sourceKey}).Scan(&id)
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				if err != nil {
					return err
				}
				current, err := readActivity(admit, tx, id, storekit.IncludeArchived)
				if err != nil {
					return err
				}
				if current.Kind != crmcontracts.ActivityKindTask {
					return apperrors.ErrConflict
				}
				found = true
				if current.ArchivedAt != nil || (current.IsDone != nil && *current.IsDone) {
					return nil
				}
				if err := auth.EnsureActivityWritableIn(admit, tx, id.UUID, true); err != nil {
					return err
				}
				if current.SourceActivityId != nil && ids.UUID(*current.SourceActivityId) != source {
					return apperrors.ErrConflict
				}
				if _, err := readActivityContent(admit, tx, ids.From[ids.ActivityKind](source), storekit.LiveOnly); err != nil {
					return err
				}
				in, err := s.inquiryArrivalPatch(bound, tx, id, source, recipient, due)
				if err != nil {
					return err
				}
				_, err = updateActivityInTx(bound, tx, id, in)
				return err
			})
		})
	}
	err := storekit.RetryLockCycles(ctx, run)
	return found, err
}

func (s *Store) inquiryArrivalPatch(ctx context.Context, tx pgx.Tx, id ids.ActivityID, source, recipient ids.UUID, due time.Time) (UpdateActivityInput, error) {
	var holderEdited, deadlineEdited bool
	// A delayed rule fills in handling details without undoing a person's choices.
	err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or("after" ? 'assignee_id'),FALSE),COALESCE(bool_or("after" ?| @deadline_fields),FALSE)
 FROM audit_log WHERE entity_type='activity' AND entity_id=@id AND actor_type=ANY(@actors)`, pgx.NamedArgs{
		"id": id.UUID, "deadline_fields": []string{"due_at", fieldDueHasTime}, "actors": []string{string(principal.PrincipalHuman), string(principal.PrincipalAgent)},
	}).Scan(&holderEdited, &deadlineEdited)
	if err != nil {
		return UpdateActivityInput{}, err
	}
	at := s.now()
	patch := UpdateActivityInput{sourceActivityID: &source, recordedAt: &at}
	if !holderEdited {
		patch.AssigneeID = new(ids.From[ids.UserKind](recipient))
	}
	if !deadlineEdited {
		patch.DueAt, patch.DueHasTime = &due, new(true)
	}
	return patch, nil
}
