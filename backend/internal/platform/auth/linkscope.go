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
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// The activity_link columns, one per record kind it can point at. Named
// because both walks over that table — the visible one here and the writable
// one beside it — spell the same five, and two lists of literals are two
// chances to spell one of them wrong.
const (
	contactIDColumn = "contact_id"
	companyIDColumn = "company_id"
	dealIDColumn    = "deal_id"
	leadIDColumn    = "lead_id"
	projectIDColumn = "project_id"
)

// LinkTargetVisibleClause answers, for ONE activity_link row, whether the
// record it points at is visible under the caller's row scope. An empty
// string means a caller for whom every target is visible — which, since
// contact and company carry capture privacy, is the system principal
// alone.
//
// It exists because "may I read this activity" and "may I be told what this
// activity is about" are different questions. The activity gate above is an
// ANY-link rule: an activity reachable through one visible contact is
// readable in full. Projecting its link rows back to the client would then
// hand over the ids of the OTHER records it touches — a colleague's deal,
// say — which the caller may not read. So the projection carries its own
// per-row predicate, built from the same disjunction the gate uses, because
// scope policy has exactly one spelling (ADR-0054 §8).
//
// alias names the activity_link table in the caller's query.
func LinkTargetVisibleClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	return linkTargetClause(ctx, alias, arg, everyLinkTarget)
}

// LinkTargetReadableClause is LinkTargetVisibleClause for a read that shows
// the caller a linked record. It also drops a link whose target type the
// caller holds no read grant on. Every seat's scope reads deal, so scope alone
// would name deals to a role without deal read. The visible clause stays for a
// row-scope question asked for somebody else, such as a SeatPrincipal.
func LinkTargetReadableClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	return linkTargetClause(ctx, alias, arg, func(table string) bool { return ReadGranted(ctx, table) })
}

func linkTargetClause(ctx context.Context, alias string, arg func(any) int, counted func(table string) bool) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if UnboundedFor(p, linkTargetTables...) {
		return "", nil
	}
	return linkTargetVisible(p, alias, arg, counted), nil
}

// everyLinkTarget counts every link type, for the row-scope-only clauses.
func everyLinkTarget(string) bool { return true }

// The row-scoped record types, named once. Several of these appear in
// half a dozen table-name positions across the package, and a typo in any
// of them silently renders a predicate that matches nothing.
const (
	tableContact = "contact"
	tableCompany = "company"
	tableDeal    = "deal"
	tableLead    = "lead"
	tableProject = "project"
)

// linkTargetTables names every record type an activity_link points at, in
// the order the disjunction below walks them. Both this projection and the
// activity gate (ActivityDiscoverClause, inheritedscope.go) decide whether they
// may skip their clause by asking UnboundedFor (rowscope.go) over this set, so
// a record type that gains capture privacy tightens both at once.
var linkTargetTables = []string{tableContact, tableCompany, tableDeal, tableLead, tableProject}

// linkTargetVisible renders the per-arm "this link's target is visible"
// disjunction over activity_link's polymorphic columns. A type counted
// rejects gets no arm, so a link of that type matches nothing.
func linkTargetVisible(p principal.Principal, alias string, arg func(any) int, counted func(table string) bool) string {
	arms := make([]string, 0, len(linkTargetTables))
	for _, t := range []struct{ column, table, probe string }{
		{contactIDColumn, tableContact, "sp"},
		{companyIDColumn, tableCompany, "so"},
		{dealIDColumn, tableDeal, "sd"},
		{leadIDColumn, tableLead, "sl"},
		{projectIDColumn, tableProject, "spr"},
	} {
		if !counted(t.table) {
			continue
		}
		arms = append(arms, linkTargetArm(alias, t.column, t.table, t.probe,
			VisiblePredicate(p, t.table, arg)(t.probe)))
	}
	if len(arms) == 0 {
		return sqlNoRow
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

// EnsureLinkTarget verifies a link's target exists, is live and is visible to
// the caller. The foreign key carries no row scope, so a guessed id would
// otherwise persist a link to a record the caller may not read.
//
// It takes FOR SHARE so an archive cannot commit between probe and insert.
// Share conflicts with the archive but not with another reference.
func EnsureLinkTarget(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if recordkinds.IsExtensionName(table) {
		return ensureExtensionRecord(ctx, tx, table, id, true)
	}
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

// ensureExtensionRecord is the row probe for a kind an extension declared, live
// or not as the caller asks. A caller without the kind's read right gets the
// answer a missing row gets, so the probe does not disclose that the id exists.
// The share lock is the one a core target takes: an archive between the probe
// and the write must wait.
func ensureExtensionRecord(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, live bool) error {
	declared, ok := recordkinds.Lookup(kind)
	if !ok || Require(ctx, kind, principal.ActionRead) != nil {
		return apperrors.ErrNotFound
	}
	query := `SELECT 1 FROM ` + pgx.Identifier{"ext", declared.Table}.Sanitize() + ` WHERE id = $1`
	if live {
		query += ` AND archived_at IS NULL`
	}
	var held int
	err := tx.QueryRow(ctx, query+` FOR SHARE`, id).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return err
}

// EnsureAttachTarget is EnsureLinkTarget for the attach direction. A record
// that gains an activity, file, membership or tag is written to, so a `read`
// share alone is refused. A `write` grant, and the own and team arms, pass.
//
// An invisible record answers not found. A record seen through a read-only
// share answers permission denied, because the share told them it exists.
func EnsureAttachTarget(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	// Visibility first, and it also takes the FOR SHARE lock every caller of
	// this probe needs before it writes — the same pairing EnsureWritable makes
	// with its own visibility probe, for the same reason.
	if err := EnsureLinkTarget(ctx, tx, table, id); err != nil {
		return err
	}
	// An extension kind has no read-only share to narrow: the kind's read right
	// the link probe just checked is all the reach there is.
	if recordkinds.IsExtensionName(table) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause, err := AttachClauseFor(ctx, table, "", arg)
	if err != nil {
		return err
	}
	if clause == "" {
		// No narrowing to apply: the caller reads every row of this table, so
		// the visibility probe above already answered everything this one could.
		return nil
	}
	var permitted bool
	if err := tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $%d AND %s)`, table, idPos, clause),
		args...).Scan(&permitted); err != nil {
		return err
	}
	if !permitted {
		return apperrors.ErrPermissionDenied
	}
	return nil
}
