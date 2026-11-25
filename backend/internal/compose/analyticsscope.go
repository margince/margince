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
// Two decisions live here. Totals: every seat that reads deals may measure the
// whole company or any live team, so the figures agree for all of them.
// Owners: which owners a seat may measure one by one (resolveOwnerScope) still
// follows the principal's own row-scope tier. Every owner-keyed output narrows
// to it through the owner gate. Assertions about a population (a
// forecast call, a share) stay on the strict tier rules in ResolveAssertionScope.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/scopesubjects"
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
	Kind string    `json:"kind"`
	ID   *ids.UUID `json:"id,omitempty"`
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
	col := paramOwnerID
	if alias != "" {
		col = alias + "." + paramOwnerID
	}
	return analyticsPopulationExpression(ctx, tx, requested, col, arg, unowned)
}

// The expression is a trusted catalog column, allowing event-time attribution
// to use the same scope authority as current ownership.
func analyticsPopulationExpression(ctx context.Context, tx pgx.Tx, requested RequestedScope, col string, arg func(any) int, unowned unownedPopulation) (ResolvedScope, string, error) {
	resolved, err := ResolveAnalyticsScope(ctx, tx, requested)
	if err != nil {
		return ResolvedScope{}, "", err
	}
	p, ok := principal.Actor(ctx)
	if !ok {
		return ResolvedScope{}, "", errors.New("compose: no actor bound to context")
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
			"%s IN (%s)", col, teamMembersSQL(arg(*resolved.ID), "= $%d")), nil
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
			col, me, col, teamMembersSQL(teams, "= ANY($%d)"))
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

// teamMembersSQL selects the members of a live team whose work its numbers
// count, for a team id that match compares.
func teamMembersSQL(pos int, match string) string {
	return fmt.Sprintf(`SELECT tm.user_id FROM `+teamMembershipFrom()+` WHERE tm.team_id `+match, pos)
}

// teamMembershipFrom is who a team's numbers count, read by its totals and by
// the floor. Those are the members of a live team who can act, and those who
// left but are not archived. A departed colleague's open deals stay in the
// totals until somebody takes them over. Every screen marks that owner as left.
func teamMembershipFrom() string {
	return `team_membership tm
		JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
		JOIN app_user u ON u.id = tm.user_id
		 AND ((` + identity.LiveMemberSQL("u") + `) OR (` + identity.DepartedMemberSQL("u") + `))`
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
	lens := analyticsLens(p)
	if requested.Kind == "" {
		return defaultScopeFor(p, lens)
	}

	if part, sentence := scopeShapeFault(requested); part != "" {
		return ResolvedScope{}, fmt.Errorf("%s: %w", sentence, apperrors.ErrInvalidArgument)
	}
	switch requested.Kind {
	case ScopeKindTeam:
		return resolveTeamScope(*requested.ID), nil
	case ScopeKindOwner:
		return resolveOwnerScope(ctx, tx, p, lens, *requested.ID)
	default:
		return ResolvedScope{Kind: ScopeKindWorkspace, Label: workspaceLabel}, nil
	}
}

// scopeShapeFault names the part of a requested scope that does not fit its
// kind ("kind" or "id") and says why, or answers "" for a scope that fits. A
// door names the part by its own wire spelling.
func scopeShapeFault(requested RequestedScope) (part, sentence string) {
	switch requested.Kind {
	case ScopeKindWorkspace:
		if requested.ID != nil {
			return "id", "the workspace scope names no subject"
		}
	case ScopeKindTeam:
		if requested.ID == nil {
			return "id", "a team scope names which team"
		}
	case ScopeKindOwner:
		if requested.ID == nil {
			return "id", "an owner scope names which owner"
		}
	default:
		return "kind", fmt.Sprintf("%q is not a population; choose owner, team or workspace", requested.Kind)
	}
	return "", ""
}

// workspaceLabel is what the whole-installation population is called. Held
// here rather than translated per caller: the label rides on the answer, and
// an answer's population must read the same in a share as on the screen.
const workspaceLabel = "Whole company"

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

// analyticsLens is the row-scope tier a seat measures by. An unbounded seat
// measures as one that reads everything.
func analyticsLens(p principal.Principal) principal.RowScope {
	if auth.Unbounded(p) {
		return principal.RowScopeAll
	}
	return p.Permissions.RowScope
}

// readsManagedTeams answers whether "My teams" is this seat's default population.
func readsManagedTeams(p principal.Principal) bool {
	resolved, err := defaultScopeFor(p, analyticsLens(p))
	return err == nil && resolved.Kind == ScopeKindManagedTeams
}

func ownScope(p principal.Principal) ResolvedScope {
	me := p.UserID
	return ResolvedScope{Kind: ScopeKindOwner, ID: &me}
}

// measuredPopulation names, to a recorder, the records a measurement covers
// when the caller's lens refuses them.
const measuredPopulation = "measured_population"

// resolveTeamScope accepts any team for totals; labelScope refuses an archived
// or unknown one as absent.
func resolveTeamScope(id ids.UUID) ResolvedScope {
	return ResolvedScope{Kind: ScopeKindTeam, ID: &id}
}

// ResolveAssertionScope answers the population a caller may ASSERT about
// (record a forecast, issue a share). Totals are open to every seat that reads
// deals. A standing number is a commitment, so the workspace needs row scope
// all and a team needs membership.
func ResolveAssertionScope(
	ctx context.Context, tx pgx.Tx, requested RequestedScope,
) (ResolvedScope, error) {
	resolved, err := ResolveAnalyticsScope(ctx, tx, requested)
	if err != nil {
		return ResolvedScope{}, err
	}
	p, ok := principal.Actor(ctx)
	if !ok {
		return ResolvedScope{}, errors.New("compose: no actor bound to context")
	}
	if err := assertionLensRefusal(p, requested, resolved); err != nil {
		return ResolvedScope{}, auth.RefuseRow(ctx, measuredPopulation, err)
	}
	return resolved, nil
}

// assertionLensRefusal is the strict tier rule for asserting about a
// population, decidable without a database.
func assertionLensRefusal(p principal.Principal, requested RequestedScope, resolved ResolvedScope) error {
	lens := analyticsLens(p)
	switch resolved.Kind {
	case ScopeKindWorkspace:
		if requested.Kind != "" && lens != principal.RowScopeAll {
			return fmt.Errorf(
				"the whole workspace is outside what you may measure: %w", apperrors.ErrPermissionDenied)
		}
	case ScopeKindTeam:
		if lens == principal.RowScopeOwn {
			return fmt.Errorf("no team is within what you may measure: %w", apperrors.ErrNotFound)
		}
		if lens == principal.RowScopeTeam && !containsID(p.TeamIDs, *resolved.ID) {
			return fmt.Errorf("that team is not one of yours: %w", apperrors.ErrNotFound)
		}
	}
	return nil
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
		shares, err := scopesubjects.SharesATeam(ctx, tx, p.TeamIDs, id)
		if err != nil {
			return ResolvedScope{}, err
		}
		if !shares {
			return ResolvedScope{}, auth.RefuseRow(ctx, measuredPopulation, fmt.Errorf(
				"that contact is not in one of your teams: %w", apperrors.ErrNotFound))
		}
	default:
		return ResolvedScope{}, auth.RefuseRow(ctx, measuredPopulation, fmt.Errorf(
			"only your own records are within what you may measure: %w", apperrors.ErrNotFound))
	}
	if err := requireMeasurableSeat(ctx, tx, id); err != nil {
		return ResolvedScope{}, err
	}
	return ResolvedScope{Kind: ScopeKindOwner, ID: &id}, nil
}

func containsID(haystack []ids.UUID, needle ids.UUID) bool {
	return slices.Contains(haystack, needle)
}

// labelScope names the resolved population for the reader.
//
// Separate from the decision above so the policy — which population, and which
// refusal — is decidable without a database, and so the naming happens exactly
// once however the scope was arrived at.
func labelScope(ctx context.Context, tx pgx.Tx, resolved *ResolvedScope) error {
	switch resolved.Kind {
	case ScopeKindOwner:
		label, err := scopesubjects.OwnerLabel(ctx, tx, *resolved.ID)
		if err != nil {
			return err
		}
		resolved.Label = label
	case ScopeKindTeam:
		label, err := scopesubjects.TeamLabel(ctx, tx, *resolved.ID)
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
