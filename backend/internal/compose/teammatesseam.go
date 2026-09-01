// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Are these two people teammates?
//
// One question with two askers. The Worklist asks it to decide whether a
// team-scoped reader may open a named person's queue; coaching asks it to
// decide whether a lead may put a notice in somebody's queue. Both are the same
// question about the same edge, so they read the same answer through this one
// seam rather than each deriving membership for itself — two derivations would
// drift, and the pair that drifts here is "may I see your day" against "may I
// speak into it".
//
// identity owns team_membership. The typed ids stay inside that module; the
// callers speak raw uuids, exactly as the row-scope seams do.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

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
