// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAReadSharedWithAnotherPrincipalRequiresBothGrants(t *testing.T) {
	reader := func(allowed bool) principal.Principal {
		id := ids.NewV7()
		return principal.Principal{
			Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + id.String(), UserID: id,
			Permissions: principal.Permissions{
				RowScope: principal.RowScopeOwn,
				Objects:  map[string]principal.ObjectGrant{"contact": {Read: allowed}},
			},
		}
	}
	for _, tc := range []struct {
		name          string
		caller, other bool
	}{
		{"both allow", true, true},
		{"caller refuses", false, true},
		{"other refuses", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), reader(tc.caller))
			err := RequireReadForReaders(ctx, "contact", reader(tc.other))
			if tc.caller && tc.other {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("shared read refusal = %v", err)
			}
		})
	}
}

// A clause grows with its readers, so it refuses more than one group and the
// groups split any list into clauses that fit.
func TestAReaderClauseTakesAtMostOneGroup(t *testing.T) {
	reader := func() principal.Principal {
		id := ids.NewV7()
		return principal.Principal{
			Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + id.String(), UserID: id,
			Permissions: principal.Permissions{
				RowScope: principal.RowScopeOwn,
				Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
			},
		}
	}
	ctx := principal.WithActor(context.Background(), reader())
	readers := make([]principal.Principal, 3*MaxReadersPerClause+1)
	for i := range readers {
		readers[i] = reader()
	}
	arg := func(any) int { return 1 }
	if _, err := ScopeClauseForReaders(ctx, "contact", "p", arg, readers...); err == nil {
		t.Fatalf("a clause over %d readers was built", len(readers))
	}
	judged := 0
	for group := range ReaderGroups(readers) {
		if _, err := ScopeClauseForReaders(ctx, "contact", "p", arg, group...); err != nil {
			t.Fatalf("a group of %d readers: %v", len(group), err)
		}
		judged += len(group)
	}
	if judged != len(readers) {
		t.Errorf("the groups judged %d readers, want all %d", judged, len(readers))
	}
}
