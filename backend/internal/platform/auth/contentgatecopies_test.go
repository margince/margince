// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type activityGate func(context.Context, string, func(any) int) (string, error)

// The census check and the per-statement tests count gates by their mark. A
// gate that rendered it twice, or none, would make every such count wrong.
func TestEveryContentGateRendersItsMarkOnce(t *testing.T) {
	gates := map[string]struct {
		render activityGate
		marks  int
	}{
		"content":  {ActivityContentClause, 1},
		"audience": {ActivityAudienceArm, 1},
		"discover": {ActivityDiscoverClause, 0},
		"readable set": {func(ctx context.Context, _ string, arg func(any) int) (string, error) {
			return ContentReadableActivitiesSQL(ctx, "SELECT id FROM activity", arg)
		}, 1},
	}
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll} {
		ctx := principal.WithActor(context.Background(), human(scope))
		for name, gate := range gates {
			var args []any
			sql, err := gate.render(ctx, "a", func(v any) int { args = append(args, v); return len(args) })
			if err != nil {
				t.Fatalf("%s gate under row scope %s: %v", name, scope, err)
			}
			if got := ContentGateCopies(sql); got != gate.marks {
				t.Errorf("the %s gate under row scope %s renders %d marks, want %d", name, scope, got, gate.marks)
			}
		}
	}
}

// Postgres keeps a plan of every clause copy on each pooled connection. So the
// content gate tests internal mail once per row it reads. Those rows are the
// activity and the message its task evidence names.
func TestTheContentGateTestsInternalMailOncePerRowItReads(t *testing.T) {
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll} {
		ctx := principal.WithActor(context.Background(), human(scope))
		var args []any
		sql, err := ActivityContentClause(ctx, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			t.Fatalf("content gate under row scope %s: %v", scope, err)
		}
		if got := strings.Count(sql, "FROM app_user internal_reader"); got != 2 {
			t.Errorf("the content gate under row scope %s tests internal mail %d times, want 2", scope, got)
		}
	}
}
