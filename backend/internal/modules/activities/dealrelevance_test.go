// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An omitted deal_id is refused by name, before any lookup reads the zero id.
func TestEveryRequiredDealRelevanceBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).SetDealRelevance(context.Background(), ids.From[ids.ActivityKind](ids.NewV7()), ids.Nil, "unrelated")
	fault, _ := httperr.Classify(err)
	if fault.Status != 422 || len(fault.Fields) != 1 || fault.Fields[0].Field != "deal_id" || fault.Fields[0].Code != "required" {
		t.Fatalf("an omitted deal_id was answered %+v, want a 422 naming deal_id", fault)
	}
}
