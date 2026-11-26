// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnalyticsScopeShapesNameTheWireField(t *testing.T) {
	actor := actorWithScope(principal.RowScopeAll)
	id := ids.NewV7()
	for _, tc := range []struct {
		scope RequestedScope
		field string
	}{
		{RequestedScope{Kind: "unknown"}, "scope_kind"},
		{RequestedScope{Kind: ScopeKindWorkspace, ID: &id}, "scope_id"},
		{RequestedScope{Kind: ScopeKindTeam}, "scope_id"},
		{RequestedScope{Kind: ScopeKindOwner}, "scope_id"},
	} {
		_, err := resolveAnalyticsScope(t.Context(), nil, actor, tc.scope)
		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 || fault.Fields[0].Field != tc.field {
			t.Errorf("scope %+v: fault %+v (%v), want 422 naming %s", tc.scope, fault, err, tc.field)
		}
	}
}
