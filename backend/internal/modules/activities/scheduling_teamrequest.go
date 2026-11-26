// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"hash/maphash"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamRequestStanding is a colleague's request on a live team link as it
// stands now. Fingerprint changes whenever its words or its answer do.
type TeamRequestStanding struct {
	Request     TeamRequest
	Pending     bool
	Fingerprint int64
}

// ReadTeamRequestTx reads the acting colleague's request on the team link page,
// so a delayed message about it says what is true when it leaves.
func ReadTeamRequestTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID) (TeamRequestStanding, error) {
	return teamRequestStandingTx(ctx, tx, page, member, "")
}

// HoldTeamRequestTx is ReadTeamRequestTx holding the link until commit, so an
// answer or a removal waits for the send reserved against it.
func HoldTeamRequestTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID) (TeamRequestStanding, error) {
	return teamRequestStandingTx(ctx, tx, page, member, " FOR SHARE OF p")
}

// teamRequestSeed keys the fingerprint. A read and its hold run in one process.
var teamRequestSeed = maphash.MakeSeed()

func teamRequestStandingTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID, lock string) (TeamRequestStanding, error) {
	if actor, ok := principal.Actor(ctx); !ok || actor.UserID != member.UUID {
		return TeamRequestStanding{}, apperrors.ErrPermissionDenied
	}
	standing := TeamRequestStanding{Request: TeamRequest{Page: page, Member: member}}
	var stored *storedSchedulingProfile
	var name string
	args := schedulingArgs{}
	err := tx.QueryRow(ctx, `SELECT p.host_user_id, p.scheduling_policy, u.display_name FROM booking_page p JOIN app_user u ON u.id = p.host_user_id
 WHERE p.id=`+args.add(page)+` AND p.revoked_at IS NULL`+lock, args...).Scan(&standing.Request.Host, &stored, &name)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && stored == nil) {
		return standing, nil
	}
	if err != nil {
		return standing, fmt.Errorf("activities: reading a colleague's booking link request: %w", err)
	}
	members := teamMembers(stored.SchedulingProfile)
	at := slices.IndexFunc(members, func(m crmcontracts.SchedulingTeamMember) bool { return memberUser(m) == member })
	if at < 0 {
		return standing, nil
	}
	state := memberState(members[at])
	standing.Request.HostName, standing.Request.Title = schedulingDisplayName(name), stored.shownTitle()
	standing.Pending = state == crmcontracts.SchedulingTeamMemberStatePending
	words := strings.Join([]string{page.String(), string(state), standing.Request.HostName, standing.Request.Title}, "\x00")
	standing.Fingerprint = int64(maphash.String(teamRequestSeed, words) >> 1)
	return standing, nil
}
