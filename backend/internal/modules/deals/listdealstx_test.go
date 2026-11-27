// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The refusal comes before the transaction is touched, so a nil one proves the
// offset was never read past.
func TestACompositeDealReadRefusesAnOffset(t *testing.T) {
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"custom"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true}},
		},
	})
	_, _, err := (&Store{}).ListDealsTx(reader, nil, ListDealsInput{Offset: new(1)}, CustomColumns{})
	if refused, ok := errors.AsType[*datasource.OffsetUnsupportedError](err); !ok || refused.Type != datasource.EntityDeal {
		t.Fatalf("a composite deal read at an offset answered %v, want the offset refusal", err)
	}
}
