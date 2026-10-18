// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// requestedDecider uses explicit responsibility, not permission to decide.
func requestedDecider(a row) ids.UUID {
	if approvalsubject.PayloadOwned(a.Kind) {
		return approvalsubject.Owner(a.ProposedChange)
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
