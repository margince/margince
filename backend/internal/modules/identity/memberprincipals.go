// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// MemberPrincipalsTx is MemberPrincipal for several members, read on the
// caller's transaction so the answer is the one that holds under the caller's
// locks. A member who is not live is absent from the answer.
func MemberPrincipalsTx(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]principal.Principal, error) {
	return principalsForTx(ctx, tx, users, LiveMemberSQL(""))
}

func principalsForTx(ctx context.Context, tx pgx.Tx, users []ids.UUID, standing string) (map[ids.UUID]principal.Principal, error) {
	rows, err := tx.Query(ctx, `SELECT id, seat_type FROM app_user WHERE id = ANY($1) AND `+standing, users)
	if err != nil {
		return nil, fmt.Errorf("identity: reading members' seats: %w", err)
	}
	type seat struct {
		ID   ids.UUID
		Seat string
	}
	seats, err := pgx.CollectRows(rows, pgx.RowToStructByPos[seat])
	if err != nil {
		return nil, fmt.Errorf("identity: reading members' seats: %w", err)
	}
	live := make([]ids.UUID, len(seats))
	for i, s := range seats {
		live[i] = s.ID
	}
	grants, err := loadGrantsFor(ctx, tx, live)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]principal.Principal, len(seats))
	for _, s := range seats {
		g := grants[s.ID]
		rbac := authz.RBAC{Permissions: g.perms, TeamIDs: rawTeamIDs(g.teams)}
		out[s.ID] = authz.HumanPrincipal(s.ID, rbac, principal.SeatType(s.Seat))
	}
	return out, nil
}

// MemberPrincipalTx is MemberPrincipalsTx for one member, and false when the
// member is not live.
func MemberPrincipalTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (principal.Principal, bool, error) {
	members, err := MemberPrincipalsTx(ctx, tx, []ids.UUID{user})
	if err != nil {
		return principal.Principal{}, false, err
	}
	member, live := members[user]
	return member, live, nil
}

// WorkPrincipalTx reads assignment authority, including an invited human whose
// work waits for sign-in. It does not authorize a session or a provider grant.
func WorkPrincipalTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (principal.Principal, bool, error) {
	members, err := principalsForTx(ctx, tx, []ids.UUID{user}, workforce.AskableMemberSQL(""))
	if err != nil {
		return principal.Principal{}, false, err
	}
	member, eligible := members[user]
	return member, eligible, nil
}
