// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAssignedTaskTransactionReadRequiresItsSnapshotClock(t *testing.T) {
	assignee := ids.From[ids.UserKind](ids.NewV7())
	_, _, err := ListActivitiesTx(unscopedCtx(), nil, ListActivitiesInput{AssigneeID: &assignee})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("missing task clock: %v", err)
	}
}
