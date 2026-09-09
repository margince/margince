// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A predicate on a REFERENCE column is a question about the record it names.
//
// `deal.company_id eq <uuid>` compiles to a comparison under the deal's
// own scope, and every seat of the workspace reads every deal. So the rows that
// come back answer "is this company on our books, and which deals are its" for
// a company the same caller's ordinary read would refuse — capture privacy on
// company, own/team scope on project. Hydration masks the id on the way
// out, which makes the answer quieter without making it different: the row is
// still there, and the predicate that selected it is still the caller's own.
//
// The TRAVERSAL form of the same question is already scoped — lateralHop runs
// branchScope over the hop record. This is the direct-field form of the hop,
// and it gets the same clause from the same place, so the two arms of one
// question cannot answer differently.
//
// Scoped on the ROW's value rather than on the id the caller sent, which is
// what makes it operator-agnostic: `ne`, `in` and a range each disclose the
// binding by which rows they leave out, and only a guard on the row itself
// covers all of them.

import (
	"context"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// referenceGuard is the extra conjunct a predicate on a reference column
// carries, or "" for a column that names nothing row-scoped.
//
// A NULL reference passes. The row names no record, so there is no target to
// be admitted to — and without this arm `company_id is_null` would answer
// nothing at all, having asked the guard about a row that has no company by
// the caller's own choice of question.
func referenceGuard(
	ctx context.Context, entity string, field Field, expr, alias string, arg func(any) int,
) (string, error) {
	branch, ok := referencedBranch(entity, field)
	if !ok {
		return "", nil
	}
	scope, admitted, err := branchScope(ctx, branch, alias, arg)
	if err != nil {
		return "", err
	}
	// Object RBAC refuses the record type outright. The reference then names
	// something this caller may not read at all, so only the rows naming
	// nothing survive — the same answer the row scope gives when it admits no
	// row, reached one gate earlier.
	if !admitted {
		return storekit.SQLf("%s IS NULL", expr), nil
	}
	// Every row of that table is readable, so there is nothing to narrow and
	// the EXISTS would only re-prove the foreign key.
	if scope == "" {
		return "", nil
	}
	return storekit.SQLf(
		"(%s IS NULL OR EXISTS (SELECT 1 FROM %s %s WHERE %s.id = %s AND %s))",
		expr, branch.table, alias, alias, expr, scope), nil
}

// referencedBranch resolves the record type a reference field names.
//
// Three spellings, because the schema has three and a resolver that knew only
// the first would read green over the other two — which is how `partner_company_id`
// stayed unguarded while `company_id` beside it was closed:
//
//   - the contract's own, `<record type>_id`, which is also what
//     contractRelations traverses on, so the direct form and the hop agree
//     about which columns are references;
//   - a NESTED member, `employer.company_id`, whose target is spelled in
//     its last segment and whose leading path names the member rather than a
//     record type;
//   - a ROLE, `partner_company_id`, which names what the reference is FOR instead
//     of what it points at. Those cannot be derived from the name at all and
//     are declared below.
//
// A branch that reads workspace-wide is not excluded here: branchScope answers
// it with an empty clause, and letting it through keeps ONE place deciding what
// a record type's row scope is.
func referencedBranch(entity string, field Field) (searchBranch, bool) {
	if field.Kind != KindID || !strings.HasSuffix(field.Name, relationSuffix) {
		return searchBranch{}, false
	}
	column := field.Name
	if _, member, nested := strings.Cut(column, memberPathSeparator); nested {
		column = lastMember(member)
	}
	if selfReferences[column] {
		return branchFor(entity)
	}
	if target, declared := roleNamedReferences[column]; declared {
		return branchFor(target)
	}
	return branchFor(strings.TrimSuffix(column, relationSuffix))
}

// lastMember answers the final segment of a dotted member path.
func lastMember(path string) string {
	for {
		_, rest, nested := strings.Cut(path, memberPathSeparator)
		if !nested {
			return path
		}
		path = rest
	}
}

// memberPathSeparator joins a nested contract member to its parent.
const memberPathSeparator = "."

// roleNamedReferences are the columns that name what a reference is FOR rather
// than what it points at. A deal's `partner_company_id` and a company's
// `parent_company_id` are both companies, and no derivation from the name can
// say so.
//
// Declared here and checked against the schema's own foreign keys by
// TestEveryQueryableReferenceResolvesToItsRecordType, so a role-named column
// added later fails rather than quietly resolving to nothing — which is the
// only way this list can be wrong and the only way that would not be noticed.
var roleNamedReferences = map[string]string{
	"partner_company_id":         entityCompany,
	"parent_company_id":          entityCompany,
	"counterparty_company_id":    entityCompany,
	"promoted_person_id":     entityPerson,
	"qualified_deal_id":      entityDeal,
	"converted_from_lead_id": entityLead,
	"source_activity_id":     entityActivity,
}

// selfReferences name another row of the SAME record type, so their target is
// whatever record carries them: a person's `merged_into_id` is a person, an
// company's is a company. Resolving them from the name would need
// one entry per record type saying the same thing.
var selfReferences = map[string]bool{"merged_into_id": true}

// refAlias names one guard's subquery. Each predicate gets its own, because a
// plan may filter on two references at once and a repeated alias would make the
// second EXISTS read the first's row.
func refAlias(n int) string {
	return "ref" + strconv.Itoa(n)
}
