// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The activity_link visibility rules. An activity has no owner of its own —
// its free-text inherits the sensitivity of the records it attaches to — so
// two different questions are answered from the same disjunction, and both
// live here so they cannot drift apart (ADR-0054 §8: scope policy has
// exactly one spelling).
//
//   - MAY I READ THIS ACTIVITY: yes if ANY of its links points at a record
//     I can see (ActivityDiscoverClause, inheritedscope.go), and MAY I
//     READ IT when its audience is limited (ActivityContentClause).
//   - MAY I BE TOLD WHAT IT IS ABOUT: per link, because the any-link answer
//     above does not license disclosing the other records it touches.
//   - MAY I POINT A LINK AT THIS RECORD: EnsureLinkTarget, the write-side
//     question, which the FK cannot answer because it is checked as the table
//     owner and carries no row scope at all.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LinkTargetVisibleClause answers, for ONE activity_link row, whether the
// record it points at is visible under the caller's row scope. An empty
// string means a caller for whom every target is visible — which, since
// person and organization carry capture privacy, is the system principal
// alone.
//
// It exists because "may I read this activity" and "may I be told what this
// activity is about" are different questions. The activity gate above is an
// ANY-link rule: an activity reachable through one visible person is
// readable in full. Projecting its link rows back to the client would then
// hand over the ids of the OTHER records it touches — a colleague's deal,
// say — which the caller may not read. So the projection carries its own
// per-row predicate, built from the same disjunction the gate uses, because
// scope policy has exactly one spelling (ADR-0054 §8).
//
// alias names the activity_link table in the caller's query.
func LinkTargetVisibleClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if UnboundedFor(p, linkTargetTables...) {
		return "", nil
	}
	return linkTargetVisible(p, alias, arg), nil
}

// The row-scoped record types, named once. Several of these appear in
// half a dozen table-name positions across the package, and a typo in any
// of them silently renders a predicate that matches nothing.
const (
	tablePerson       = "person"
	tableOrganization = "organization"
	tableDeal         = "deal"
	tableLead         = "lead"
	tableProject      = "project"
)

// linkTargetTables names every record type an activity_link points at, in
// the order the disjunction below walks them. Both this projection and the
// activity gate (ActivityDiscoverClause, inheritedscope.go) decide whether they
// may skip their clause by asking UnboundedFor (rowscope.go) over this set, so
// a record type that gains capture privacy tightens both at once.
var linkTargetTables = []string{tablePerson, tableOrganization, tableDeal, tableLead, tableProject}

// linkTargetVisible renders the per-arm "this link's target is visible"
// disjunction over activity_link's polymorphic columns.
func linkTargetVisible(p principal.Principal, alias string, arg func(any) int) string {
	arms := make([]string, 0, len(linkTargetTables))
	for _, t := range []struct{ column, table, probe string }{
		{"person_id", tablePerson, "sp"},
		{"organization_id", tableOrganization, "so"},
		{"deal_id", tableDeal, "sd"},
		{"lead_id", tableLead, "sl"},
		{"project_id", tableProject, "spr"},
	} {
		arms = append(arms, linkTargetArm(alias, t.column, t.table, t.probe,
			VisiblePredicate(p, t.table, arg)(t.probe)))
	}
	return "(\n\t      " + strings.Join(arms, "\n\t   OR ") + ")"
}

// linkTargetArm renders one arm of that disjunction. A predicate of TRUE —
// the caller reads every row of that table — collapses the arm to the
// column test alone: the composite FK already guarantees the target row
// exists, so the EXISTS could only confirm what TRUE already said.
//
// Dropping it is not a micro-optimization. The walk runs per candidate
// row, and the search branch runs it across a whole FTS result set: an
// all-scope reader was paying five correlated joins to be told yes five
// times, which measured as a 6x regression on the search_fts perf budget
// the moment capture privacy stopped exempting them from the walk.
func linkTargetArm(alias, column, table, probe, predicate string) string {
	if predicate == "TRUE" {
		return fmt.Sprintf(`%s.%s IS NOT NULL`, alias, column)
	}
	return fmt.Sprintf(`(%[1]s.%[2]s IS NOT NULL AND EXISTS (SELECT 1 FROM %[3]s %[4]s WHERE %[4]s.id = %[1]s.%[2]s AND %[5]s))`,
		alias, column, table, probe, predicate)
}

// EnsureLinkTarget verifies an activity link's target row exists AND is
// visible to the caller — an explicit row-scope probe, because the FK that
// would otherwise catch a bad id is checked as the table owner and carries
// no row scope at all: it accepts any id that exists, so without this a
// guessed foreign UUID would persist a link to a record the caller may not
// read. A link to an archived record is equally refused: the link would
// outlive the row it names.
//
// It HOLDS the row it answers about, and that is what separates it from
// EnsureVisibleLive. Every caller writes the reference afterwards, in the same
// transaction: probe, then insert. Read unlocked, an archive committing in
// between makes the answer stale before it is used, and the reference lands on
// a record that is no longer live — the TOCTOU storekit.LockRow's own doc warns
// against, on the target side. Under READ COMMITTED the lock is also the
// re-check: a waiter wakes on the archived row and the liveness filter refuses
// it, rather than acting on what it read before waiting.
//
// FOR SHARE, and the pairing is the point. It conflicts with the archive, which
// UPDATEs the row, while two references onto one record do not conflict and have
// no reason to queue behind each other — a person mid-ingest is referenced by
// every message captured for them. The same pairing the activity_link trigger
// takes on the activity it is about (migration 1788000100).
//
// Taken in the probe rather than at each write, unlike LockSubjectLive next door
// in writescope.go. That lock is exclusive and would add an edge to two dozen
// documented lock orders at once; this one is shared, so it orders nothing
// against another reference and only ever waits for a writer of the very row
// being referenced. Against that, forty call sites would each have to remember,
// and the one that forgot would look exactly like the thirty-nine that did not.
//
// The few read paths that call this for its existence half — a contract listing
// under an organization, a relationship read under its anchor — pay for it too:
// they hold that one anchor for the length of a short read, which delays an
// archive of it and blocks nothing else. That is the price of the probe having
// one meaning.
func EnsureLinkTarget(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := ScopeClauseFor(ctx, table, "", arg)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`SELECT 1 FROM %s WHERE id = $%d AND archived_at IS NULL`, table, idPos)
	if clause != "" {
		q += " AND " + clause
	}
	q += " FOR SHARE"

	var held int
	if err := tx.QueryRow(ctx, q, args...).Scan(&held); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}
