// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"testing"
)

func TestLeadTaskReadsRefuseAnUnpreparedIdentitySnapshot(t *testing.T) {
	t.Parallel()
	arg := func(any) int { t.Fatal("unprepared identities must not bind a guessed map"); return 0 }
	if _, err := leadColumns(context.Background(), arg); err == nil {
		t.Fatal("columns accepted missing identities")
	}
	if _, err := orderByLeadNextTaskDue(context.Background(), arg); err == nil {
		t.Fatal("sort accepted missing identities")
	}
}
