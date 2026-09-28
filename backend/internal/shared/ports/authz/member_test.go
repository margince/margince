// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// oneSnapshot answers only through AdmittedAuthority; the two single reads
// fail the test, so a loader that reads the seat apart from the grants shows.
type oneSnapshot struct {
	t    *testing.T
	seat principal.SeatType
	err  error
}

func (r oneSnapshot) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	r.t.Error("MemberPrincipal read the grants apart from the seat")
	return authz.RBAC{}, nil
}

func (r oneSnapshot) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	r.t.Error("MemberPrincipal read the seat apart from the grants")
	return "", nil
}

func (r oneSnapshot) AdmittedAuthority(_ context.Context, _, _, passport ids.UUID) (authz.RBAC, principal.SeatType, error) {
	if !passport.IsZero() {
		r.t.Errorf("a member acting for themselves presented passport %s", passport)
	}
	return authz.RBAC{
		Permissions: principal.Permissions{RowScope: principal.RowScopeTeam},
		TeamIDs:     []ids.UUID{ids.NewV7()},
	}, r.seat, r.err
}

func TestAMemberPrincipalCarriesTheSeatReadWithTheGrants(t *testing.T) {
	t.Parallel()
	member := ids.NewV7()
	p, err := authz.MemberPrincipal(context.Background(), oneSnapshot{t: t, seat: principal.SeatRead}, ids.NewV7(), member)
	if err != nil {
		t.Fatalf("MemberPrincipal: %v", err)
	}
	if p.SeatType != principal.SeatRead {
		t.Errorf("seat = %q, want read: a read seat lost on the way is a principal that may write", p.SeatType)
	}
	if p.Type != principal.PrincipalHuman || p.UserID != member || p.ID != "human:"+member.String() {
		t.Errorf("principal = %+v, want the member as a human acting for themselves", p)
	}
	if p.Permissions.RowScope != principal.RowScopeTeam || len(p.TeamIDs) != 1 {
		t.Errorf("grants = %+v teams %v, want the resolver's", p.Permissions, p.TeamIDs)
	}
}

func TestAMemberWhoIsGoneHasNoPrincipal(t *testing.T) {
	t.Parallel()
	_, err := authz.MemberPrincipal(context.Background(),
		oneSnapshot{t: t, err: apperrors.ErrNotFound}, ids.NewV7(), ids.NewV7())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: absence of authority is denial, never empty permission", err)
	}
}
