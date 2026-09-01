// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who are my teammates?
//
// One question with three askers, in two shapes. The Worklist asks the yes/no
// shape to decide whether a team-scoped reader may open a named person's queue;
// coaching asks the same one to decide whether a lead may put a notice in
// somebody's queue; the team board asks the enumerating shape to draw its rows.
// All three read through this one seam rather than each deriving membership for
// itself — two derivations would drift, and the pair that drifts here is "may I
// see your day" against "may I speak into it".
//
// The two shapes answer from the same predicate inside identity, which is the
// property the board depends on: a name this seam lists is a queue the same seam
// will open.
//
// identity owns team_membership. The typed ids stay inside that module; the
// callers speak raw uuids, exactly as the row-scope seams do.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type teammatesSeam struct{ svc *identity.Service }

// newTeammatesSeam builds the membership reader over the identity module.
func newTeammatesSeam(pool *pgxpool.Pool) teammatesSeam {
	return teammatesSeam{svc: identity.NewService(pool)}
}

// SharesLiveTeamWithCaller carries no caller argument on purpose: identity reads
// the asker from the principal, so neither consumer can ask about an edge its
// caller is not an end of.
func (t teammatesSeam) SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error) {
	return t.svc.SharesLiveTeamWithCaller(ctx, ids.From[ids.UserKind](other))
}

// LiveTeammatesOfCaller carries no caller argument for the same reason: identity
// reads the asker from the principal, so nobody can enumerate a team they are
// not on.
//
// The email address identity returns is dropped here. The board prints a name
// and links by id, and carrying an address it never draws would publish every
// colleague's address through a surface with no reason to hold one.
func (t teammatesSeam) LiveTeammatesOfCaller(ctx context.Context) ([]attention.TeamMember, error) {
	members, err := t.svc.LiveTeammatesOfCaller(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]attention.TeamMember, 0, len(members))
	for _, member := range members {
		out = append(out, attention.TeamMember{
			UserID:      member.UserID,
			DisplayName: member.DisplayName,
		})
	}
	return out, nil
}
