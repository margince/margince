// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// archived_at is no column of custom_field, yet a client may still sort by it.
func TestTheArchivedAtSortIsAcceptedAndKeepsTheHouseOrder(t *testing.T) {
	t.Parallel()
	spec := "-archived_at"
	sorted, err := storekit.ParseListSort(context.Background(), &spec, catalogSortFields, func(any) int { return 1 })
	if err != nil {
		t.Fatalf("the archived_at sort was refused: %v", err)
	}
	if got := sorted.OrderBy(); !strings.HasPrefix(got, " ORDER BY NULL::timestamptz DESC") || !strings.HasSuffix(got, "created_at DESC, id DESC") {
		t.Errorf("the archived_at sort orders by %q, want a constant null ahead of the house order", got)
	}
}
