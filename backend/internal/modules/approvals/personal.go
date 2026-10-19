// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"slices"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// requestedDecider uses explicit responsibility, not permission to decide.
func requestedDecider(a row) ids.UUID {
	if approvalsubject.PayloadOwned(a.Kind) {
		return approvalsubject.Owner(a.ProposedChange)
	}
	if a.PassportID != nil && !a.PassportID.IsZero() && a.OnBehalfOf != nil {
		return a.OnBehalfOf.UUID
	}
	if !selfOnlyKinds[a.Kind] && !decidedByTheSeatStagedFor[a.Kind] && !stagedForStagerOnly(a.TargetType, a.TargetID != nil) {
		return ids.Nil
	}
	if a.OnBehalfOf == nil {
		return ids.Nil
	}
	return a.OnBehalfOf.UUID
}

func requestedFor(a row, reader ids.UUID) bool {
	return !reader.IsZero() && requestedDecider(a) == reader
}

func requestedInScope(a row, reader ids.UUID, in ListInput) bool {
	if in.RequestedForReader && !requestedFor(a, reader) {
		return false
	}
	owner := requestedDecider(a)
	if in.OnlyUnassigned && !owner.IsZero() {
		return false
	}
	return in.RequestedForOwners == nil || slices.Contains(in.RequestedForOwners, owner)
}
