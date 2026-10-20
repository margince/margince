// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"cmp"
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type ActivityRelinkPreview struct {
	Activity             crmcontracts.Activity
	Removed              []ids.UUID
	Participants         []RelinkParticipantChange
	AddsLink             bool
	StampsCorrespondence bool
}

// The set is explicit: a growing thread cannot describe a fixed approval scope.
func (s *Store) PreviewRelink(ctx context.Context, activityIDs []ids.UUID, in RelinkActivityInput) ([]ActivityRelinkPreview, error) {
	if err := validateRelinkCount(len(activityIDs)); err != nil {
		return nil, err
	}
	if len(activityIDs) > 1 {
		if err := refuseBatchPin(in); err != nil {
			return nil, err
		}
	}
	column, err := admitRelink(ctx, in)
	if err != nil {
		return nil, err
	}
	ordered := slices.Clone(activityIDs)
	slices.SortFunc(ordered, func(a, b ids.UUID) int { return cmp.Compare(a.String(), b.String()) })
	ordered = slices.Compact(ordered)
	var out []ActivityRelinkPreview
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureAttachTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
			return err
		}
		for _, raw := range ordered {
			plan, err := previewRelinkRow(ctx, tx, ids.From[ids.ActivityKind](raw), in, column)
			if err != nil {
				return err
			}
			out = append(out, plan)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func previewRelinkRow(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, column string) (ActivityRelinkPreview, error) {
	var out ActivityRelinkPreview
	if err := LockActivityLive(ctx, tx, id); err != nil {
		return out, err
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, true); err != nil {
		return out, err
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return out, err
	}
	if err := relinkMeetsItsPin(ctx, tx, id, in.IfVersion, false); err != nil {
		return out, err
	}
	activity, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return out, err
	}
	if err := CheckActivityLinkKind(string(activity.Kind), ActivityLinkInput{EntityType: in.EntityType, EntityID: in.EntityID}); err != nil {
		return out, err
	}
	existing, err := visibleRelinkTargets(ctx, tx, id, in.EntityType, column)
	if err != nil {
		return out, err
	}
	out.Activity = activity
	out.AddsLink = in.ReplaceExistingOfType || !slices.Contains(existing, in.EntityID)
	if in.ReplaceExistingOfType {
		out.Removed = existing
	}
	if in.ReplaceExistingOfType && in.EntityType == linkEntityContact {
		out.Participants, err = previewRelinkParticipants(ctx, tx, id, in.EntityID, existing)
		if err != nil {
			return ActivityRelinkPreview{}, err
		}
	}
	out.StampsCorrespondence = out.AddsLink && in.EntityType == linkEntityProject
	return out, nil
}
