// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which population an analytics answer is ABOUT, as opposed to which records
// the caller may legally open.
//
// Those are two different questions and the tree already answers the second
// one: auth.ScopeClauseFor renders the row scope, and for a deal it renders
// TRUE, because deals are workspace-readable by design (tableclass.go) so two
// reps do not call the same buyer. That is right for a record read and wrong
// for a measurement. A rep asking "how much pipeline do I have" who is handed
// the workspace total has been answered a question nobody asked.
//
// So a population predicate is applied ON TOP of the row scope, never instead
// of it. Both narrow; neither widens. The row scope says what may be seen, this
// says what was asked about, and an answer is the intersection.
//
// The lens comes from the principal's own row-scope tier rather than from a
// role name the frontend sent: own reads own, team reads its teams plus
// itself, all reads the workspace. A requested scope outside the lens is
// refused here, once, so every analytics surface refuses it the same way.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The population kinds, spelled as the wire spells them.
const (
	ScopeKindWorkspace = "workspace"
	ScopeKindTeam      = "team"
	ScopeKindOwner     = "owner"
)

// RequestedScope is what the caller asked for. A zero value means they asked
// for nothing, which resolves to their lens default rather than to the
// workspace — the distinction this whole file exists to make.
type RequestedScope struct {
	Kind string
	ID   *ids.UUID
}

// ResolvedScope is what the server decided, and what every answer must quote
// back. Label is server-authored: a client that resolved a team id into a name
// itself would be naming a team it may not read.
type ResolvedScope struct {
	Kind  string
	ID    *ids.UUID
	Label string
}

// ResolveAnalyticsScope answers the population this caller may measure, or
// refuses.
//
// The resolution WITHOUT the SQL, for the paths that need the authority answer
// and have nothing to narrow: a forecast call asserts a standing number ABOUT a
// population rather than reading rows from it, and a share records which
// population a link is for. Both were taking the caller's word for it.
//
// Exported beside the clause builder rather than duplicated inside those paths,
// and AnalyticsPopulationClause calls it, so there is one answer to "may this
// seat measure that" and a write path cannot come to a different one than the
// read path beside it.
func ResolveAnalyticsScope(
	ctx context.Context, tx pgx.Tx, requested RequestedScope,
) (ResolvedScope, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return ResolvedScope{}, errors.New("compose: no actor bound to context")
	}
	resolved, err := resolveAnalyticsScope(ctx, tx, p, requested)
	if err != nil {
		return ResolvedScope{}, err
	}
	if err := labelScope(ctx, tx, &resolved); err != nil {
		return ResolvedScope{}, err
	}
	return resolved, nil
}

// AnalyticsPopulationClause resolves a requested scope against the caller's
// lens and renders the SQL that narrows to it.
//
// unownedPopulation says whether an owner/team CALLER's default population
// (never an explicitly named one — see the ScopeKindOwner/ScopeKindTeam
// split in AnalyticsPopulationClause) admits a row nobody owns yet.
//
// A REPORT and a STANDING FORECAST answer this differently for the same
// reason they answer row-scope's identical question differently
// (platform/auth/rowscope.go's unownedRows, unownedIsShared/unownedIsNobodys):
// a report reads what is on a caller's radar, and an unrouted lead sitting in
// their worklist belongs on their board too. A forecast is a commitment
// number, and nobody has committed to a deal nobody has claimed — widening it
// would make a manager's team total stop reconciling with the sum of naming
// each member by id, the one thing a standing forecast promises never to do.
type unownedPopulation bool

const (
	unownedIsPartOfDefault unownedPopulation = true
	unownedIsExcluded      unownedPopulation = false
)

// AnalyticsPopulationClause resolves a requested scope against the caller's
// lens and renders the SQL that narrows to it.
//
// The returned clause is a bare predicate WITHOUT a leading AND, so a caller
// composes it the way they compose any other; an empty string means the
// population is the whole workspace and nothing needs adding.
//
// alias is interpolated into SQL and must be a compile-time literal from the
// calling spec, never a name off a request.
func AnalyticsPopulationClause(
	ctx context.Context, tx pgx.Tx, requested RequestedScope, alias string, arg func(any) int,
	unowned unownedPopulation,
) (ResolvedScope, string, error) {
	resolved, err := ResolveAnalyticsScope(ctx, tx, requested)
	if err != nil {
		return ResolvedScope{}, "", err
	}
	p, ok := principal.Actor(ctx)
	if !ok {
		return ResolvedScope{}, "", errors.New("compose: no actor bound to context")
	}

	col := paramOwnerID
	if alias != "" {
		col = alias + "." + paramOwnerID
	}

	switch resolved.Kind {
	case ScopeKindOwner:
		clause := fmt.Sprintf("%s = $%d", col, arg(*resolved.ID))
		if unowned == unownedIsPartOfDefault && *resolved.ID == p.UserID {
			// Measuring MYSELF — whether I asked for that by name or asked for
			// nothing — is the one case `platform/auth.OwnerPredicate` already
			// answers for reads: "a row nobody owns is the workspace's to see."
			// A population predicate narrows what a READ already permitted, so
			// an unowned row that is on my worklist has to be on my population
			// too, or a rep's own board disagrees with a rep's own list about a
			// lead sitting in both.
			//
			// Naming somebody ELSE by id does not get this arm even when the
			// caller admits unowned rows for their own default: that is a
			// standing forecast or report asking what a NAMED colleague
			// specifically committed to, and an unclaimed deal is nobody's
			// commitment yet.
			clause = ownedOrUnowned(col, clause)
		}
		return resolved, clause, nil
	case ScopeKindTeam:
		// An EXPLICITLY named team (never the default — see ScopeKindManagedTeams
		// below) is the same "somebody specific" accountability ask ScopeKindOwner
		// makes for a named colleague, so it stays exact for the same reason,
		// regardless of unowned.
		return resolved, fmt.Sprintf(
			"%s IN (%s)", col, liveTeamMembersSQL(arg(*resolved.ID), "= $%d")), nil
	case ScopeKindManagedTeams:
		// A team manager who named nothing is asking about the work they are
		// accountable for, which is their teams AND themselves. Themselves
		// explicitly: a manager carrying deals of their own is not a member of
		// their own team in every installation, and dropping their pipeline
		// from their own default answer reads as data loss.
		me := arg(p.UserID)
		teams := arg(p.TeamIDs)
		clause := fmt.Sprintf(
			"(%s = $%d OR %s IN (%s))",
			col, me, col, liveTeamMembersSQL(teams, "= ANY($%d)"))
		if unowned == unownedIsPartOfDefault {
			// Always the caller's own default population (never requestable by
			// name, see ScopeKindManagedTeams's own doc), so the same
			// unowned-is-shared arm as the self-named ScopeKindOwner case above
			// applies.
			clause = ownedOrUnowned(col, clause)
		}
		return resolved, clause, nil
	default:
		return resolved, "", nil
	}
}

// ownedOrUnowned widens an owner/team population clause to also admit a row
// nobody owns yet.
//
// Mirrors platform/auth.OwnerPredicate's unownedIsShared arm for row scope,
// for the same reason: an owner_id IS NULL row matches neither `= $n` nor
// `IN (...)` under ordinary SQL null semantics, so without this a fresh,
// unrouted lead or an unassigned deal silently drops out of every report a
// non-admin seat runs, though the SAME row is on their ordinary, unscoped
// list. The two clauses cannot share one function — this one narrows to an
// ARBITRARY resolved population, that one always narrows to the caller's own
// — so the invariant is kept in one place in each file's own doc instead: a
// reader changing what "unowned" means to a read has to find this one too.
func ownedOrUnowned(col, clause string) string {
	return fmt.Sprintf("(%s IS NULL OR %s)", col, clause)
}

// liveTeamMembersSQL selects the members of a team, as the installation stands
// today: a live team, and a member who can still act.
//
// Membership rows deliberately outlive both — an archived team keeps its rows so
// a restore brings them back, and deactivating a seat leaves the row alone. A
// predicate reading team_membership by itself therefore measures contacts who left,
// which is the defect this replaces: the resolver refuses a departed colleague as
// a population of their own while their deals went on counting inside their old
// team's total.
func liveTeamMembersSQL(pos int, match string) string {
	return fmt.Sprintf(`SELECT tm.user_id FROM team_membership tm
		JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
		JOIN app_user u ON u.id = tm.user_id AND `+identity.LiveMemberSQL("u")+`
		WHERE tm.team_id `+match, pos)
}

// ScopeKindManagedTeams is the resolved-only kind for "my teams and me". It is
// never accepted from the wire: a caller names a single team, or names nothing
// and gets this. Keeping it out of the request vocabulary means a client cannot
// ask for somebody else's managed set.
const ScopeKindManagedTeams = "managed_teams"

// resolveAnalyticsScope decides the population, refusing anything the lens does
// not cover.
//
// Refusals are apperrors.ErrNotFound rather than permission-denied wherever the
// subject is a team or a user the caller may not measure: 403 on a specific id
// confirms that id exists, which is the disclosure the lens was for.
func resolveAnalyticsScope(
	ctx context.Context, tx pgx.Tx, p principal.Principal, requested RequestedScope,
) (ResolvedScope, error) {
	lens := p.Permissions.RowScope
	if auth.Unbounded(p) {
		lens = principal.RowScopeAll
	}

	if requested.Kind == "" {
		return defaultScopeFor(p, lens)
	}

	switch requested.Kind {
	case ScopeKindWorkspace:
		if lens != principal.RowScopeAll {
			return ResolvedScope{}, fmt.Errorf(
				"the whole workspace is outside what you may measure: %w", apperrors.ErrPermissionDenied)
		}
		if requested.ID != nil {
			return ResolvedScope{}, fmt.Errorf(
				"the workspace scope names no subject: %w", apperrors.ErrInvalidArgument)
		}
		return ResolvedScope{Kind: ScopeKindWorkspace, Label: workspaceLabel}, nil

	case ScopeKindTeam:
		if requested.ID == nil {
			return ResolvedScope{}, fmt.Errorf(
				"a team scope names which team: %w", apperrors.ErrInvalidArgument)
		}
		return resolveTeamScope(p, lens, *requested.ID)

	case ScopeKindOwner:
		if requested.ID == nil {
			return ResolvedScope{}, fmt.Errorf(
				"an owner scope names which owner: %w", apperrors.ErrInvalidArgument)
		}
		return resolveOwnerScope(ctx, tx, p, lens, *requested.ID)

	default:
		return ResolvedScope{}, fmt.Errorf(
			"%q is not a population: %w", requested.Kind, apperrors.ErrInvalidArgument)
	}
}

// workspaceLabel is what the whole-installation population is called. Held
// here rather than translated per caller: the label rides on the answer, and
// an answer's population must read the same in a share as on the screen.
const workspaceLabel = "Whole workspace"

func defaultScopeFor(p principal.Principal, lens principal.RowScope) (ResolvedScope, error) {
	switch lens {
	case principal.RowScopeAll:
		return ResolvedScope{Kind: ScopeKindWorkspace, Label: workspaceLabel}, nil
	case principal.RowScopeTeam:
		// A team-lens principal with no teams reads as own: a manager of
		// nothing measures themselves rather than the installation.
		if len(p.TeamIDs) == 0 {
			return ownScope(p), nil
		}
		return ResolvedScope{Kind: ScopeKindManagedTeams, Label: managedTeamsLabel}, nil
	default:
		return ownScope(p), nil
	}
}

const managedTeamsLabel = "My teams"

func ownScope(p principal.Principal) ResolvedScope {
	me := p.UserID
	return ResolvedScope{Kind: ScopeKindOwner, ID: &me}
}

func resolveTeamScope(p principal.Principal, lens principal.RowScope, id ids.UUID) (ResolvedScope, error) {
	if lens == principal.RowScopeOwn {
		return ResolvedScope{}, fmt.Errorf(
			"no team is within what you may measure: %w", apperrors.ErrNotFound)
	}
	if lens == principal.RowScopeTeam && !containsID(p.TeamIDs, id) {
		return ResolvedScope{}, fmt.Errorf(
			"that team is not one of yours: %w", apperrors.ErrNotFound)
	}
	return ResolvedScope{Kind: ScopeKindTeam, ID: &id}, nil
}

func resolveOwnerScope(
	ctx context.Context, tx pgx.Tx, p principal.Principal, lens principal.RowScope, id ids.UUID,
) (ResolvedScope, error) {
	// Yourself is always within your own lens, including for a rep, and it
	// costs no membership read.
	if id == p.UserID {
		return ownScope(p), nil
	}
	switch lens {
	case principal.RowScopeAll:
	case principal.RowScopeTeam:
		shares, err := sharesATeamWith(ctx, tx, p.TeamIDs, id)
		if err != nil {
			return ResolvedScope{}, err
		}
		if !shares {
			return ResolvedScope{}, fmt.Errorf(
				"that contact is not in one of your teams: %w", apperrors.ErrNotFound)
		}
	default:
		return ResolvedScope{}, fmt.Errorf(
			"only your own records are within what you may measure: %w", apperrors.ErrNotFound)
	}
	return ResolvedScope{Kind: ScopeKindOwner, ID: &id}, nil
}

// sharesATeamWith asks whether one user is a live member of any of these teams.
//
// The membership read is what makes a team manager's owner scope safe: without
// it a manager could name any user id in the installation and measure them.
func sharesATeamWith(ctx context.Context, tx pgx.Tx, teams []ids.UUID, user ids.UUID) (bool, error) {
	if len(teams) == 0 {
		return false, nil
	}
	var shares bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM team_membership tm
		JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
		JOIN app_user u ON u.id = tm.user_id AND `+identity.LiveMemberSQL("u")+`
		WHERE tm.user_id = $1 AND tm.team_id = ANY($2))`, user, teams).Scan(&shares)
	if err != nil {
		return false, fmt.Errorf("compose: reading team membership: %w", err)
	}
	return shares, nil
}

func containsID(haystack []ids.UUID, needle ids.UUID) bool {
	for _, id := range haystack {
		if id == needle {
			return true
		}
	}
	return false
}

// labelScope names the resolved population for the reader.
//
// Separate from the decision above so the policy — which population, and which
// refusal — is decidable without a database, and so the naming happens exactly
// once however the scope was arrived at.
func labelScope(ctx context.Context, tx pgx.Tx, resolved *ResolvedScope) error {
	switch resolved.Kind {
	case ScopeKindOwner:
		label, err := analyticsUserLabel(ctx, tx, *resolved.ID)
		if err != nil {
			return err
		}
		resolved.Label = label
	case ScopeKindTeam:
		label, err := analyticsTeamLabel(ctx, tx, *resolved.ID)
		if err != nil {
			return err
		}
		resolved.Label = label
	case ScopeKindManagedTeams:
		resolved.Label = managedTeamsLabel
	default:
		resolved.Label = workspaceLabel
	}
	return nil
}

// analyticsUserLabel and analyticsTeamLabel name a population for the reader.
//
// Both refuse an archived subject as absent rather than labelling it. A scope
// resolved against a team that was archived after the request was written is
// not a population any more, and answering over it would report a set the
// installation no longer has.

func analyticsUserLabel(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	var label string
	err := tx.QueryRow(ctx,
		`SELECT display_name FROM app_user WHERE id = $1 AND `+identity.LiveMemberSQL(""), id).Scan(&label)
	if errors.Is(err, pgx.ErrNoRows) {
		// Deactivated counts as absent, not merely archived: a departed
		// colleague is no longer a population somebody measures.
		return "", fmt.Errorf("no such contact to measure: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("compose: naming the measured contact: %w", err)
	}
	return label, nil
}

func analyticsTeamLabel(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	var label string
	err := tx.QueryRow(ctx,
		`SELECT name FROM team WHERE id = $1 AND archived_at IS NULL`, id).Scan(&label)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("no such team to measure: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("compose: naming the measured team: %w", err)
	}
	return label, nil
}
