// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// TeamLinksDataTask moves each team a personal page names onto a team link of
// its own. The personal page then offers its host's own free time again. The
// store's team request notice asks each member still pending on the new link.
func TeamLinksDataTask(store *Store, now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1795540594_teams_move_to_team_links", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795540594_teams_move_to_team_links"),
		Requires: []string{"core/1795540594"},
		Steps: []datatask.Step{{
			Name: "move_teams", Limit: 100, MinLimit: 5,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: CountPersonalPagesWithTeams, Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return store.MoveTeamsToTeamLinksTx(ctx, tx, in, now())
			},
			EmptyWhenDone: true,
		}},
	}
}

// personalPageWithTeamSQL is a live personal page that still names a team.
const personalPageWithTeamSQL = `revoked_at IS NULL AND NOT team_link
 AND jsonb_typeof(scheduling_policy->'team_members') = 'array' AND scheduling_policy->'team_members' <> '[]'::jsonb`

// CountPersonalPagesWithTeams counts the personal pages whose team is still to move.
func CountPersonalPagesWithTeams(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM booking_page WHERE `+personalPageWithTeamSQL+` ORDER BY id LIMIT @limit) p`,
		pgx.NamedArgs{argLimit: maximum + 1}).Scan(&count)
	if err != nil {
		return 0, false, fmt.Errorf("activities: counting personal pages with a team: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}

// MoveTeamsToTeamLinksTx moves one batch of teams. Each member keeps their own
// answer, and the personal page keeps its address, alias and settings.
func (s *Store) MoveTeamsToTeamLinksTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, now time.Time) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM booking_page WHERE id > @after AND `+personalPageWithTeamSQL+` ORDER BY id LIMIT @limit`,
		pgx.NamedArgs{repairAfterArg: after, argLimit: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: paging personal pages with a team: %w", err)
	}
	pages, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, page := range pages {
		moved, err := s.moveTeamToLinkTx(ctx, tx, page, now)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if moved {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, pages, changed)
}

// moveTeamToLinkTx gives a personal page's team a new team link with a new
// address, then clears the team from the personal page. A member still pending
// is asked again on the new link, which waits for them. It answers false when
// the page no longer names a team.
func (s *Store) moveTeamToLinkTx(ctx context.Context, tx pgx.Tx, page ids.UUID, now time.Time) (bool, error) {
	stored, err := lockLivePageTx(ctx, tx, page)
	if errors.Is(err, apperrors.ErrNotFound) || (err == nil && (stored == nil || len(teamMembers(stored.SchedulingProfile)) == 0)) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var host ids.UserID
	var used *time.Time
	args := schedulingArgs{}
	if err := tx.QueryRow(ctx, `SELECT host_user_id, first_used_at FROM booking_page WHERE id=`+args.add(page)+` AND NOT team_link`, args...).Scan(&host, &used); err != nil {
		return false, err
	}
	if used == nil {
		used = &now
	}
	slug, err := mintBookingSlug(rand.Read)
	if err != nil {
		return false, err
	}
	link := storedSchedulingProfile{Title: stored.shownTitle(), Enabled: stored.Enabled, TeamMembers: stored.TeamMembers}
	var linkID ids.UUID
	args = schedulingArgs{}
	if err := tx.QueryRow(ctx, `INSERT INTO booking_page (host_user_id, slug, team_link, scheduling_policy, first_used_at) VALUES (`+
		args.add(host)+`, `+args.add(slug)+`, true, `+args.add(link)+`, `+args.add(*used)+`) RETURNING id`, args...).Scan(&linkID); err != nil {
		return false, err
	}
	audit, err := storekit.AuditEvent(ctx, tx, "create", "booking_page", linkID, link)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, audit, linkID, crmcontracts.InternalEventBookingPageUpdated{Enabled: link.Enabled}); err != nil {
		return false, err
	}
	pending := slices.DeleteFunc(slices.Clone(teamMembers(link.SchedulingProfile)), func(m crmcontracts.SchedulingTeamMember) bool {
		return memberState(m) != crmcontracts.SchedulingTeamMemberStatePending
	})
	if err := s.tellTeamChanges(ctx, tx, host, linkID, nil, crmcontracts.SchedulingProfile{Title: link.Title, TeamMembers: &pending}); err != nil {
		return false, err
	}
	before := teamAudit{TeamMembers: teamMembers(stored.SchedulingProfile)}
	stored.TeamMembers = nil
	args = schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET scheduling_policy=`+args.add(stored)+` WHERE id=`+args.add(page), args...); err != nil {
		return false, err
	}
	audit, err = storekit.Audit(ctx, tx, "update", "booking_page", page, before, teamAudit{TeamMembers: []crmcontracts.SchedulingTeamMember{}})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: stored.Enabled})
}
