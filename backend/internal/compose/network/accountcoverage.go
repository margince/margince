// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// How well we know an ACCOUNT, as against one deal.
//
// The deal grain already answers "is this deal single-threaded" (risk.go). The
// account grain asks it across every deal and project a company is on, which is
// the question a manager reviewing a book of business actually has.
//
// WHY A HIDDEN CONTACT IS NOT AN ABSENT ONE. A stakeholder can be a
// capture-private person — one a connector minted from somebody's mailbox and
// no human has promoted. The deal-grain reader makes those seats ABSENT, and
// that is right for a list: an invisible seat reads as an empty one, exactly as
// every other row-scoped list answers. It is wrong for a COUNT. An account with
// three stakeholders a reader cannot open would count zero and be reported
// single-threaded — a fact about that reader's permissions, printed as a fact
// about the customer.
//
// So this counts both: the stakeholders it can name, and the ones it cannot.
// When anything was withheld the verdict is `unknown`, never `single_threaded`.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ThreadingVerdict is what this reader is willing to say about an account.
type ThreadingVerdict string

const (
	// ThreadingMultiple is enough named, distinct stakeholders to clear the
	// threading floor. Safe to say even with contacts withheld: seeing enough
	// already answers the question, and the hidden ones can only add.
	ThreadingMultiple ThreadingVerdict = "multi_threaded"
	// ThreadingSingle is a genuine finding: the reader can see EVERY
	// stakeholder this account has, and there are too few.
	ThreadingSingle ThreadingVerdict = "single_threaded"
	// ThreadingUnknown is too few visible AND something withheld. The account
	// may be well covered by people this reader cannot open, and saying
	// "single-threaded" would report a permission as a business fact.
	ThreadingUnknown ThreadingVerdict = "unknown"
	// ThreadingNoContacts is the honest empty: nothing withheld, and no
	// stakeholder recorded at all. Distinct from single-threaded, which needs
	// at least somebody, and from unknown, which needs something hidden.
	ThreadingNoContacts ThreadingVerdict = "no_observed_contacts"
)

// AccountCoverage is one company's relationship breadth.
type AccountCoverage struct {
	OrganizationID ids.UUID
	// Stakeholders the reader may open, deduplicated across every deal and
	// project of this account. A person on three deals is one relationship.
	VisibleStakeholders []ids.UUID
	// CoverageIncomplete says some stakeholder of this account is one this
	// reader cannot open. A BOOLEAN and never a count: the verdict below only
	// ever asks whether anything was withheld, and an exact number is an oracle
	// — a caller watching it move from 2 to 3 has learned that a colleague
	// captured a new contact at this account, which is precisely the fact
	// capture privacy exists to keep.
	CoverageIncomplete bool
	// Roles are the distinct roles among the visible stakeholders, so a reader
	// can see which side of the house is covered.
	Roles []string
	// Threading is what this reader may be told, given what they could see.
	Threading ThreadingVerdict
	// SectionsOmitted names the sections withheld for lack of the edge grant,
	// on the same terms as DealCoverage: a caller who may read no edges at all
	// is told so, rather than served an empty account that reads as a bare one.
	SectionsOmitted []string
}

// AccountCoverageFor assembles one account's relationship breadth.
//
// The edge admission comes FIRST, and a denial becomes a named omission rather
// than an empty answer — the same shape CoverageFor uses, and for the same
// reason: every stakeholder is an edge, so a caller without the grant would
// otherwise be served zero contacts and told the account is uncovered.
func AccountCoverageFor(ctx context.Context, tx pgx.Tx, orgID ids.UUID) (AccountCoverage, error) {
	out := AccountCoverage{OrganizationID: orgID}
	// The ACCOUNT first, before anything is read about it.
	//
	// Without this the function answers about any organization id a caller can
	// name, whether or not they may open it — and the incompleteness flag then
	// tells them something about a company they were never admitted to. The
	// edge admission below does not cover it: relationship.read says a caller
	// may read edges, not which accounts they may ask about.
	//
	// EnsureVisibleLive rather than Require alone, so an archived or
	// out-of-scope account is not-found rather than answered.
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "organization", orgID); err != nil {
		return out, err
	}
	if err := auth.EdgeReadAdmitted(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			out.SectionsOmitted = edgeWithheldSections()
			out.Threading = ThreadingUnknown
			return out, nil
		}
		return out, err
	}

	visible, roles, err := visibleAccountStakeholders(ctx, tx, orgID)
	if err != nil {
		return out, err
	}
	total, err := countAccountStakeholders(ctx, tx, orgID)
	if err != nil {
		return out, err
	}
	out.VisibleStakeholders, out.Roles = visible, roles
	// Strictly greater, not "different". READ COMMITTED gives each statement
	// its own snapshot, so a stakeholder added between the two reads makes the
	// total larger without anything being hidden — and a `!=` here would
	// manufacture a privacy signal out of a concurrent insert. Over-reporting
	// incompleteness costs a reader an `unknown` they did not need; the reverse
	// would cost them a confident wrong verdict, so the comparison leans this
	// way on purpose.
	out.CoverageIncomplete = total > len(visible)
	out.Threading = threadingVerdict(len(visible), out.CoverageIncomplete)
	return out, nil
}

// threadingVerdict decides what this reader may be told.
//
// The asymmetry is deliberate. Seeing ENOUGH is safe to report whatever is
// hidden, because hidden contacts can only add to the count. Seeing too few is
// only a finding when there is nothing left to hide behind — otherwise the
// honest answer is that we do not know.
func threadingVerdict(visible int, incomplete bool) ThreadingVerdict {
	if visible >= reportThreadingFloor {
		return ThreadingMultiple
	}
	if incomplete {
		return ThreadingUnknown
	}
	if visible == 0 {
		return ThreadingNoContacts
	}
	return ThreadingSingle
}

// visibleAccountStakeholders reads the people this caller may open who are
// stakeholders on any deal or project of this account, plus their roles.
//
// Deduplicated across edges: somebody on three deals of one account is one
// relationship, and counting them three times would clear the threading floor
// on a single contact.
func visibleAccountStakeholders(ctx context.Context, tx pgx.Tx, orgID ids.UUID) ([]ids.UUID, []string, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	edge, err := accountStakeholderEdge(ctx, orgPos, arg)
	if err != nil {
		return nil, nil, err
	}
	bound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, nil, err
	}
	if bound == "" {
		bound = scopeAll
	}
	// The PERSON row scope, beside the edge grant. They answer different
	// questions — may this caller read seats at all, and which of them — and a
	// caller holding both record grants and neither of these is served nothing.
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT p.id, coalesce(r.role, '')
		  FROM relationship r
		  JOIN person p ON p.id = r.person_id AND p.archived_at IS NULL
		 WHERE r.archived_at IS NULL
		   AND %s
		   AND (%s)
		   AND (%s)
		 ORDER BY p.id`, edge, bound, scope), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("network: reading an account's stakeholders: %w", err)
	}
	defer rows.Close()
	var people []ids.UUID
	seenRole := map[string]bool{}
	var roles []string
	for rows.Next() {
		var person ids.UUID
		var role string
		if err := rows.Scan(&person, &role); err != nil {
			return nil, nil, fmt.Errorf("network: reading an account's stakeholders: %w", err)
		}
		// DISTINCT is over the (person, role) pair, so one person holding two
		// roles arrives twice and is one relationship.
		if len(people) == 0 || people[len(people)-1] != person {
			people = append(people, person)
		}
		if role != "" && !seenRole[role] {
			seenRole[role] = true
			roles = append(roles, role)
		}
	}
	return people, roles, rows.Err()
}

// countAccountStakeholders counts every distinct person seated on this account,
// WITHOUT the person row scope.
//
// It is the one read here that omits it, and that omission is the whole point.
// EdgeReadScope narrows an edge by EVERY endpoint it carries, the person
// included — so a count taken under it drops exactly the capture-private
// contacts whose absence this number exists to report, and answers zero every
// time. An account would then read as single-threaded because its contacts
// belong to somebody else's mailbox.
//
// The reader is still gated: AccountCoverageFor takes the edge admission before
// this runs, and the account is named by a deal or project the caller reached
// through their own read. What is deliberately not applied is the person
// predicate, and the answer is a COUNT — no id, no name, no role. What a caller
// learns is that their own view is incomplete, which is what stops the verdict
// being wrong about the customer.
func countAccountStakeholders(ctx context.Context, tx pgx.Tx, orgID ids.UUID) (int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	edge, err := accountStakeholderEdge(ctx, orgPos, arg)
	if err != nil {
		return 0, err
	}
	var total int
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(DISTINCT r.person_id)
		  FROM relationship r
		  JOIN person p ON p.id = r.person_id AND p.archived_at IS NULL
		 WHERE r.archived_at IS NULL
		   AND %s`, edge), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("network: counting an account's stakeholders: %w", err)
	}
	return total, nil
}

// accountStakeholderEdge is the predicate selecting every stakeholder edge of
// one account: a seat on one of its deals, or on one of its projects.
//
// Both statements above render this same predicate, because the visible set and
// the total have to cover the SAME population: any difference between them
// lands in the withheld count and reads as hidden people who are not there.
func accountStakeholderEdge(ctx context.Context, orgPos int, arg func(any) int) (string, error) {
	// The deal and project the seat hangs off, under the caller's own scope for
	// each. Both statements carry this, so the ONLY difference between the
	// visible set and the total is the person — which is what makes their
	// difference mean "hidden contact" rather than "deal I cannot reach".
	//
	// Without it the total counted seats on deals outside the caller's scope,
	// and an account would report itself incomplete because somebody else's
	// team runs a deal on it.
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return "", err
	}
	if dealScope == "" {
		dealScope = scopeAll
	}
	projectScope, err := auth.ScopeClauseFor(ctx, "project", "pr", arg)
	if err != nil {
		return "", err
	}
	if projectScope == "" {
		projectScope = scopeAll
	}
	return fmt.Sprintf(`(
		   (r.kind = 'deal_stakeholder' AND EXISTS (
		      SELECT 1 FROM deal d WHERE d.id = r.deal_id
		        AND d.organization_id = $%[1]d AND d.archived_at IS NULL
		        AND (%[2]s)))
		OR (r.kind = 'project_stakeholder' AND EXISTS (
		      SELECT 1 FROM relationship pc
		        JOIN project pr ON pr.id = pc.project_id AND pr.archived_at IS NULL
		       WHERE pc.kind = 'project_company' AND pc.project_id = r.project_id
		         AND pc.organization_id = $%[1]d AND pc.archived_at IS NULL
		         AND (%[3]s))))`, orgPos, dealScope, projectScope), nil
}
