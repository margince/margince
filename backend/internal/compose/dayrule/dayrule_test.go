// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dayrule

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAReadNarrowedToTheActingMemberIsTheirOwnList(t *testing.T) {
	me, other := ids.NewV7(), ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: me})
	for name, c := range map[string]struct {
		owners *storekit.OwnerSet
		want   occurredday.Surface
	}{
		"only me":           {&storekit.OwnerSet{IDs: []ids.UUID{me}}, occurredday.Personal},
		"no narrowing":      {nil, occurredday.Combined},
		"a team":            {&storekit.OwnerSet{IDs: []ids.UUID{me, other}}, occurredday.Combined},
		"someone else":      {&storekit.OwnerSet{IDs: []ids.UUID{other}}, occurredday.Combined},
		"me and unassigned": {&storekit.OwnerSet{IDs: []ids.UUID{me}, Unassigned: true}, occurredday.Combined},
	} {
		if got := SurfaceOf(ctx, c.owners); got != c.want {
			t.Errorf("%s: surface %v, want %v", name, got, c.want)
		}
	}
	if got := SurfaceOf(context.Background(), &storekit.OwnerSet{IDs: []ids.UUID{me}}); got != occurredday.Combined {
		t.Errorf("with nobody signed in the surface is %v, want combined", got)
	}
}
