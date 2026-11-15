// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The two count sorts withhold their order exactly when the printed count is
// withheld. Both read the gate the list and the single read use, so this
// checks every combination of the four grants against that gate.
func TestCompanyCountSortsFollowTheCountGates(t *testing.T) {
	for grants := range 16 {
		t.Run(fmt.Sprintf("grants-%04b", grants), func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{
				Type: principal.PrincipalHuman,
				Permissions: principal.Permissions{
					RowScope: principal.RowScopeAll,
					Objects: map[string]principal.ObjectGrant{
						"company":        {Read: true},
						"contact":        {Read: grants&1 != 0},
						"relationship":   {Read: grants&2 != 0},
						"deal":           {Read: grants&4 != 0},
						"computed_field": {Read: grants&8 != 0},
					},
				},
			})
			wantContacts := grants&3 == 3
			wantPipeline := grants&12 == 12
			if contactCountVisible(ctx) != wantContacts {
				t.Fatalf("contactCountVisible = %v, want %v", !wantContacts, wantContacts)
			}
			if pipelineCountsVisible(ctx) != wantPipeline {
				t.Fatalf("pipelineCountsVisible = %v, want %v", !wantPipeline, wantPipeline)
			}
			assertSortWithheld(ctx, t, "contact_count", orderByContactCount, !wantContacts)
			assertSortWithheld(ctx, t, "open_deal_count", orderByOpenDealCount, !wantPipeline)
		})
	}
}

func assertSortWithheld(ctx context.Context, t *testing.T, name string,
	order func(context.Context, func(any) int) (string, error), want bool,
) {
	t.Helper()
	var args []any
	sql, err := order(ctx, func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		t.Fatalf("%s sort: %v", name, err)
	}
	if got := sql == withheldSortValue; got != want {
		t.Fatalf("%s sort withheld = %v, want %v (sql %q)", name, got, want, sql)
	}
}
