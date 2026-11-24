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

// TeamRequestStanding is a colleague's request on a host's live booking link as
// it stands now. Fingerprint changes whenever its words or its answer do.
type TeamRequestStanding struct {
	Request     TeamRequest
	Pending     bool
	Fingerprint int64
}

// ReadTeamRequestTx reads the acting colleague's request on host's link, so a
// delayed message about it says what is true when it leaves.
func ReadTeamRequestTx(ctx context.Context, tx pgx.Tx, host, member ids.UserID) (TeamRequestStanding, error) {
	return readTeamRequestTx(ctx, tx, host, member, "")
}

// HoldTeamRequestTx is ReadTeamRequestTx holding the link until commit, so an
// answer or a removal waits for the send reserved against it.
func HoldTeamRequestTx(ctx context.Context, tx pgx.Tx, host, member ids.UserID) (TeamRequestStanding, error) {
	return readTeamRequestTx(ctx, tx, host, member, " FOR SHARE OF p")
}

// teamRequestSeed keys the fingerprint. A read and its hold run in one process.
var teamRequestSeed = maphash.MakeSeed()

func readTeamRequestTx(ctx context.Context, tx pgx.Tx, host, member ids.UserID, lock string) (TeamRequestStanding, error) {
	if actor, ok := principal.Actor(ctx); !ok || actor.UserID != member.UUID {
		return TeamRequestStanding{}, apperrors.ErrPermissionDenied
	}
	standing := TeamRequestStanding{Request: TeamRequest{Host: host, Member: member}}
	var page ids.UUID
	var stored *storedSchedulingProfile
	var name string
	args := schedulingArgs{}
	err := tx.QueryRow(ctx, `SELECT p.id, p.scheduling_policy, u.display_name FROM booking_page p JOIN app_user u ON u.id = p.host_user_id
 WHERE p.host_user_id=`+args.add(host)+` AND p.revoked_at IS NULL ORDER BY p.created_at DESC LIMIT 1`+lock, args...).Scan(&page, &stored, &name)
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
	standing.Request.HostName, standing.Request.Title = schedulingDisplayName(name), stored.Title
	standing.Pending = state == crmcontracts.SchedulingTeamMemberStatePending
	words := strings.Join([]string{page.String(), string(state), standing.Request.HostName, standing.Request.Title}, "\x00")
	standing.Fingerprint = int64(maphash.String(teamRequestSeed, words)) //nolint:gosec // A fingerprint compares equal or not; the wrap changes nothing.
	return standing, nil
}
