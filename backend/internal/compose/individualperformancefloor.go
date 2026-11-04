// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The small-group floor while "Show individual performance" is off. A figure
// is withheld when it minus the reader's own figure is one colleague's. It is
// also withheld when taking away a team total the reader may read leaves one
// colleague's. Every surface reaches it through thinGroups.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fewestColleagues is how many live colleagues besides the reader must share a
// total that any colleague contributes to, while the switch is off. With one,
// the total minus the reader's own figure is that colleague's figure.
const fewestColleagues = 2

// requireNoLoneColleague refuses, while the switch is off, an answer with a
// group that other seats contribute to but fewer than fewestColleagues live
// colleagues do. Each group is the owners whose rows make up one figure. A
// departed owner's rows still count as a colleague's, and do not count toward
// the floor.
func requireNoLoneColleague(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	thin, err := thinGroups(ctx, tx, groups)
	if err != nil {
		return err
	}
	if slices.Contains(thin, true) {
		return auth.IndividualPerformanceOffError{}
	}
	return nil
}

// thinGroups answers, per group, whether it falls below the floor. Membership
// is read in the caller's transaction, so a change counts on the next request.
func thinGroups(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) ([]bool, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return nil, errors.New("compose: no actor bound to context")
	}
	others := map[ids.UUID]bool{}
	for _, group := range groups {
		for _, owner := range group {
			if !owner.IsZero() && owner != p.UserID {
				others[owner] = true
			}
		}
	}
	if len(others) == 0 {
		return make([]bool, len(groups)), nil
	}
	seats := slices.Collect(maps.Keys(others))
	live, err := liveColleagues(ctx, tx, seats)
	if err != nil {
		return nil, err
	}
	teams, err := subtractableTeams(ctx, tx, p, seats)
	if err != nil {
		return nil, err
	}
	return belowFloor(p.UserID, groups, live, teams), nil
}

// floorTeam is one total a reader may take from another: a live team, or the
// reader's own "My teams".
type floorTeam struct {
	team    ids.UUID
	managed bool
}

// belowFloor is the floor itself. A group is thin when it holds colleagues
// besides the reader but fewer than fewestColleagues live ones. It is also thin
// when taking out the members of one of teamsOf's teams leaves it so. The
// overlap per team is counted from the group's own seats, so the work grows
// with the seats, not with pairs of teams. A seat named twice counts once.
func belowFloor(reader ids.UUID, groups [][]ids.UUID, live map[ids.UUID]bool, teamsOf map[ids.UUID][]floorTeam) []bool {
	type overlap struct{ colleagues, live int }
	out := make([]bool, len(groups))
	seen := map[ids.UUID]bool{}
	shared := map[floorTeam]overlap{}
	for index, group := range groups {
		clear(seen)
		clear(shared)
		colleagues, counted := 0, 0
		for _, seat := range group {
			if seat.IsZero() || seat == reader || seen[seat] {
				continue
			}
			seen[seat] = true
			colleagues++
			if live[seat] {
				counted++
			}
			for _, team := range teamsOf[seat] {
				both := shared[team]
				both.colleagues++
				if live[seat] {
					both.live++
				}
				shared[team] = both
			}
		}
		thin := colleagues > 0 && counted < fewestColleagues
		for _, both := range shared {
			thin = thin || (colleagues > both.colleagues && counted-both.live < fewestColleagues)
		}
		out[index] = thin
	}
	return out
}

// subtractableTeams answers, for each seat, the totals that count it and that
// the reader may read. Those are every live team, and "My teams" when that is
// the reader's default.
func subtractableTeams(ctx context.Context, tx pgx.Tx, p principal.Principal, seats []ids.UUID) (map[ids.UUID][]floorTeam, error) {
	teams, err := readableTeamMembers(ctx, tx, seats)
	if err != nil {
		return nil, err
	}
	readsMyTeams := readsManagedTeams(p)
	out := make(map[ids.UUID][]floorTeam, len(teams))
	for seat, ofSeat := range teams {
		for _, team := range ofSeat {
			out[seat] = append(out[seat], floorTeam{team: team})
		}
		if readsMyTeams && slices.ContainsFunc(ofSeat, func(team ids.UUID) bool { return slices.Contains(p.TeamIDs, team) }) {
			out[seat] = append(out[seat], floorTeam{managed: true})
		}
	}
	return out, nil
}

// readableTeamMembers answers, for each of these seats, the teams whose totals
// count it. Every reader may read any live team's total, so these are the
// totals a reader can take from one another.
func readableTeamMembers(ctx context.Context, tx pgx.Tx, seats []ids.UUID) (map[ids.UUID][]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT tm.user_id, tm.team_id FROM `+teamMembershipFrom()+` WHERE tm.user_id = ANY($1)`, seats)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the teams of a figure's seats: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID][]ids.UUID{}
	for rows.Next() {
		var seat, team ids.UUID
		if err := rows.Scan(&seat, &team); err != nil {
			return nil, fmt.Errorf("compose: reading the teams of a figure's seats: %w", err)
		}
		out[seat] = append(out[seat], team)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the teams of a figure's seats: %w", err)
	}
	return out, nil
}

// liveColleagues answers which of these seats are live human members.
func liveColleagues(ctx context.Context, tx pgx.Tx, seats []ids.UUID) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.id = ANY($1) AND NOT u.is_agent AND `+
		identity.LiveMemberSQL("u"), seats)
	if err != nil {
		return nil, fmt.Errorf("compose: reading which contributors are live members: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("compose: reading which contributors are live members: %w", err)
	}
	out := make(map[ids.UUID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

// requirePopulationFloor holds a team, "My teams" or company total to the
// floor by its members, not only by who contributed. Otherwise a total equal
// to another team's would show that the one member between them sold nothing.
// An owner population is the reader's own and has nothing to hold. A capture
// the system takes has no reader; what it froze meets the floor when read.
func requirePopulationFloor(ctx context.Context, tx pgx.Tx, resolved ResolvedScope) error {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalSystem {
		return nil
	}
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	members, counted, err := populationMembers(ctx, tx, resolved.Kind, resolved.ID)
	if err != nil || !counted {
		return err
	}
	return requireNoLoneColleague(ctx, tx, [][]ids.UUID{members})
}

// requireSpecPopulationFloor is requirePopulationFloor for a report spec's
// answer. A spec without an owner column has no team to narrow to, so no team
// total to take from it.
func requireSpecPopulationFloor(ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed || !measuresOwners(spec) {
		return err
	}
	resolved, err := ResolveAnalyticsScope(ctx, tx, reportPopulationScope(spec, requested))
	if err != nil {
		return err
	}
	return requirePopulationFloor(ctx, tx, resolved)
}

// populationBelowFloor answers, while the switch is off, whether a reporting
// scope's members fall below the floor. In a team of two, a figure credited to
// the reader can be a colleague's work.
func populationBelowFloor(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, owners reporting.MeasurableOwners) (bool, error) {
	if !owners.IndividualOff {
		return false, nil
	}
	members, counted, err := populationMembers(ctx, tx, string(scope.Kind), (*ids.UUID)(scope.Id))
	if err != nil || !counted {
		return false, err
	}
	thin, err := thinGroups(ctx, tx, [][]ids.UUID{members})
	if err != nil {
		return false, err
	}
	return thin[0], nil
}

// populationMembers lists the seats a population's totals count. counted is
// false for an owner population, which has no members to list.
func populationMembers(ctx context.Context, tx pgx.Tx, kind string, id *ids.UUID) (members []ids.UUID, counted bool, err error) {
	var sql string
	var args []any
	switch kind {
	case ScopeKindTeam:
		if id == nil {
			return nil, false, nil
		}
		sql, args = teamMembersSQL(1, "= $%d"), []any{*id}
	case ScopeKindManagedTeams:
		p, ok := principal.Actor(ctx)
		if !ok {
			return nil, false, errors.New("compose: no actor bound to context")
		}
		sql, args = teamMembersSQL(1, "= ANY($%d)"), []any{p.TeamIDs}
	case ScopeKindWorkspace:
		sql = companyMembersSQL
	default:
		return nil, false, nil
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT m.user_id), '{}') FROM (`+sql+`) m`, args...).Scan(&members)
	if err != nil {
		return nil, false, fmt.Errorf("compose: reading a population's members for the floor: %w", err)
	}
	return members, true, nil
}

// companyMembersSQL selects the members whose work the company's totals count,
// on the rule a team's totals count theirs by.
var companyMembersSQL = `SELECT u.id AS user_id FROM app_user u WHERE NOT u.is_agent
	AND ((` + identity.LiveMemberSQL("u") + `) OR (` + identity.DepartedMemberSQL("u") + `))`

// targetFloor answers, while the switch is off, which team and company
// targets keep enough colleagues behind them to show. A target is in effect a
// colleague's own when its scope has other members but fewer than
// fewestColleagues live ones besides the reader. Archived teams are checked
// too, and a team the check never reads is not on the list, so it is withheld.
func targetFloor(ctx context.Context, tx pgx.Tx) (reporting.TargetFloor, error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return reporting.TargetFloor{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, array_remove(array_agg(tm.user_id), NULL) FROM team t
		LEFT JOIN team_membership tm ON tm.team_id = t.id
		GROUP BY t.id ORDER BY t.id`)
	if err != nil {
		return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
	}
	var teams []ids.UUID
	var members [][]ids.UUID
	for rows.Next() {
		var team ids.UUID
		var seats []ids.UUID
		if err := rows.Scan(&team, &seats); err != nil {
			rows.Close()
			return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
		}
		teams, members = append(teams, team), append(members, seats)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
	}
	everyone, _, err := populationMembers(ctx, tx, ScopeKindWorkspace, nil)
	if err != nil {
		return reporting.TargetFloor{}, err
	}
	thin, err := thinGroups(ctx, tx, append(members, everyone))
	if err != nil {
		return reporting.TargetFloor{}, err
	}
	floor := reporting.TargetFloor{Applies: true, Workspace: !thin[len(teams)], Teams: []ids.UUID{}}
	for index, team := range teams {
		if !thin[index] {
			floor.Teams = append(floor.Teams, team)
		}
	}
	return floor, nil
}
