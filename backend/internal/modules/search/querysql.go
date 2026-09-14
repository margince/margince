// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A validated plan, as SQL.
//
// Nothing a caller wrote reaches this statement as text. A field name is
// resolved through the vocabulary and then through the storage binding, so the
// identifier interpolated is one Postgres itself named; operands and document
// leaves bind as parameters. The identifier is sanitized on the way in anyway,
// because a defence that rests on an argument about provenance is one edit away
// from being wrong.
//
// Every read this builds carries what every other read on this surface carries:
// archived_at IS NULL, the branch's discovery narrowing, object RBAC, and the
// caller's row-scope clause — for the HOP as much as for the target, since a
// hop is a read of the record it lands on.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// hopBinding is the resolved traversal: which record type the hop lands on,
// what it can answer, and which side of the edge holds the reference.
type hopBinding struct {
	relation Relation
	branch   searchBranch
	columns  *storage
	// column is the reference column, and forward says whose it is: the
	// target's (`deal.company_id` → company) or the hop record's
	// (company → the deals that point back at it). Both are empty for a
	// join edge, which holds its two columns on relation.Join because neither
	// record carries one.
	column  string
	forward bool
}

// newHopBinding reads the edge off the relation.
//
// A JOIN edge is recognised first and by its Join member, never by its text:
// its Via is prose for a human, and a Via read for structure would find the
// dot in `relationship(contact_id → company_id)` and take a join edge for
// an inverse one — an edge that would then compile against a column the join
// table does not have.
//
// A scalar edge is read off Relation.Via, which records the contract reference
// the relation was DERIVED from in two spellings: a bare `company_id` is
// the target's own column, and a qualified `deal.company_id` is the
// referring record's.
func newHopBinding(relation Relation, branch searchBranch, columns *storage) hopBinding {
	if relation.Join != nil {
		return hopBinding{relation: relation, branch: branch, columns: columns}
	}
	column, forward := relation.Via, true
	if _, qualified, ok := strings.Cut(relation.Via, "."); ok {
		column, forward = qualified, false
	}
	return hopBinding{relation: relation, branch: branch, columns: columns, column: column, forward: forward}
}

// planBinding is everything the compiler needs that the plan itself does not
// carry: where each record type is stored and what it can answer.
type planBinding struct {
	branch  searchBranch
	columns *storage
	hop     *hopBinding
	// candidates narrows the statement to the ids the similarity lane ranked.
	// Empty means the exact lane, which is bounded by the limit instead.
	candidates []ids.UUID
	// fetch is the row ceiling for the exact lane: the limit plus one, so a
	// truncated answer is detectable rather than merely suspected.
	fetch int
	// geo is the bound radius predicate on the ROOT target, when the plan
	// carries one this deployment can answer. It is on the binding rather than
	// read from the plan because binding it needs the place cache and the
	// deployment's own columns — the same reason `columns` is here.
	//
	// Nil for every plan without a radius, which is nearly all of them.
	geo *geoBinding
}

// planCompiler accumulates the statement's bound arguments.
type planCompiler struct {
	args []any
	// refs counts the reference guards rendered so far, so each gets a subquery
	// alias of its own — two guards sharing one would make the second EXISTS
	// read the first's row.
	refs int
}

//craft:ignore naked-any a bound parameter is whatever Go type the column's kind encodes as (string, int64, float64, bool, time.Time, ids.UUID) — the kind switch IS the conversion contract, so there is no narrower signature
func (c *planCompiler) arg(v any) int {
	c.args = append(c.args, v)
	return len(c.args)
}

// compileStatement renders the statement, or admitted=false when object RBAC has
// stopped admitting a record type this plan needs. That is a mid-flight
// permission change rather than a caller error, and it answers an empty result
// for the same reason every other read on this surface does: existence-hiding
// costs nothing here and a 403 would confirm the record type is populated.
func (c *planCompiler) compileStatement(ctx context.Context, plan ValidatedPlan, binding planBinding) (string, bool, error) {
	scope, admitted, err := branchScope(ctx, binding.branch, "t", c.arg)
	if err != nil || !admitted {
		return "", false, err
	}
	where := []string{"t.archived_at IS NULL"}
	if narrowing := binding.branch.narrowing("t"); narrowing != "" {
		where = append(where, narrowing)
	}
	if scope != "" {
		where = append(where, scope)
	}
	predicates, refusals, err := c.predicates(ctx, "t", binding.columns, plan.Target, "where", plan.Plan.Where, true)
	if err != nil {
		return "", false, err
	}
	where = append(where, predicates...)

	// The radius, when the plan carries one this deployment can answer. The
	// clause path above skips it — a place has no expression to compare — so it
	// is rendered here, where the bound center and columns are in hand.
	distance := ""
	if binding.geo != nil {
		var geoWhere []string
		distance, geoWhere = c.radius("t", *binding.geo)
		where = append(where, geoWhere...)
	}

	join, hopSelect, hopRefusals, hopAdmitted, err := c.lateralHop(ctx, plan, binding)
	if err != nil || !hopAdmitted {
		return "", false, err
	}
	refusals = append(refusals, hopRefusals...)
	if len(refusals) > 0 {
		return "", false, &PlanRefusal{Refusals: refusals}
	}
	if len(binding.candidates) > 0 {
		where = append(where, c.idsIn("t.id", binding.candidates))
	}

	// The distance rides the projection so the answer can say how far, and so
	// the ORDER BY below can name it without computing it twice. It is appended
	// AFTER the hop columns, which keeps the scan targets in
	// scanPlanRows appending in the same order they are added here.
	distanceSelect := ""
	if distance != "" {
		distanceSelect = ", " + distance + " AS distance_km"
	}
	sql := fmt.Sprintf("SELECT t.id, %s AS title%s%s FROM %s t%s WHERE %s",
		binding.branch.title, hopSelect, distanceSelect, binding.branch.table, join,
		strings.Join(where, " AND "))

	// THE THIRD LANE. A radius orders by distance, nearest first, in BOTH the
	// exact and the similarity lanes — asking "within 50km" is asking about
	// nearness, so distance orders the answer and similarity only decides who
	// qualifies for it (Lars, 2026-08-21). This is the one case where the
	// similarity lane takes a SQL ORDER BY, which is why orderByRank has to
	// leave a distance-ordered answer alone.
	if distance != "" {
		return sql + fmt.Sprintf(" ORDER BY distance_km ASC, t.id DESC LIMIT $%d",
			c.arg(binding.fetch)), true, nil
	}
	if len(binding.candidates) > 0 {
		// The similarity lane is already bounded by the ranked candidate set,
		// and its order is the retriever's rather than the table's, so it is
		// applied after the rows come back rather than by the statement.
		return sql, true, nil
	}
	// Ids are uuidv7, so the primary key already orders by creation time: one
	// always-present, unique column, deterministic under concurrent writes
	// without a second sort key or a nullable column to reason about.
	return sql + fmt.Sprintf(" ORDER BY t.id DESC LIMIT $%d", c.arg(binding.fetch)), true, nil
}

// lateralHop renders the traversal as a LATERAL join that returns ONE matching
// hop row, which is what makes the hop legible as evidence rather than as an
// invisible filter. An EXISTS would answer the same rows and explain none of
// them.
//
// The hop carries its own admission and its own row scope: a caller who cannot
// see the Stuttgart company cannot use it to select deals either.
func (c *planCompiler) lateralHop(ctx context.Context, plan ValidatedPlan, binding planBinding) (
	join, selection string, refusals []apperrors.FieldRefusal, admitted bool, err error,
) {
	if binding.hop == nil {
		return "", "", nil, true, nil
	}
	hop := binding.hop
	scope, admitted, err := branchScope(ctx, hop.branch, "h", c.arg)
	if err != nil || !admitted {
		return "", "", nil, false, err
	}
	where := []string{"h.archived_at IS NULL", c.edgeCondition(*hop)}
	// The hop carries the SAME discovery narrowing the target does. A
	// traversal selects the hop record by its attributes, which is discovery
	// by any other name — so a plan asking for "deals at a company in
	// Stuttgart" must not reach the installation's own company through a door
	// the search arm keeps shut.
	if narrowing := hop.branch.narrowing("h"); narrowing != "" {
		where = append(where, narrowing)
	}
	if scope != "" {
		where = append(where, scope)
	}
	predicates, refusals, err := c.predicates(ctx, "h", hop.columns, plan.HopVocabulary, "traverse.where", plan.Plan.Traverse.Where, false)
	if err != nil {
		return "", "", nil, false, err
	}
	where = append(where, predicates...)

	// ORDER BY keeps the evidence deterministic when several hop rows match;
	// the row's membership never depends on which one is returned.
	join = fmt.Sprintf(
		" JOIN LATERAL (SELECT h.id AS hop_id, %s AS hop_title FROM %s h WHERE %s ORDER BY h.id LIMIT 1) hop ON true",
		hop.branch.title, hop.branch.table, strings.Join(where, " AND "),
	)
	return join, ", hop.hop_id, hop.hop_title", refusals, true, nil
}

// edgeCondition joins the two records on the reference the relation was
// derived from, in whichever of the three ways declares it: the target's own
// column, the referring record's, or a table between them.
func (c *planCompiler) edgeCondition(hop hopBinding) string {
	if hop.relation.Join != nil {
		return joinEdgeCondition(*hop.relation.Join)
	}
	column := sanitize(hop.column)
	if hop.forward {
		return "h.id = t." + column
	}
	return "h." + column + " = t.id"
}

// predicates renders one where-list, reporting a refusal per clause it cannot
// bind rather than the first — a caller told about one of three operand faults
// makes three round trips to learn what one answer could have carried.
// root says whether these predicates are the ones on the record being searched
// for, as opposed to a traversal's. It decides one thing: whether a radius is
// bound elsewhere (root) or has nowhere to go (a hop).
func (c *planCompiler) predicates(
	ctx context.Context, alias string, columns *storage, vocab TargetVocabulary,
	path string, clauses []Predicate, root bool,
) ([]string, []apperrors.FieldRefusal, error) {
	var (
		fragments []string
		refusals  []apperrors.FieldRefusal
	)
	for i, clause := range clauses {
		at := path + "[" + strconv.Itoa(i) + "]"
		if clause.Op == OpWithinRadius {
			// Rendered by planCompiler.radius instead, from the bound centre
			// and the deployment's coordinate columns. A place has no single
			// column to compare, so the ordinary `field op value` path below
			// cannot express it — it would refuse with unknown_field.
			//
			// ONLY ON THE ROOT TARGET. The same loop compiles a traversal's
			// predicates, and there the radius is NOT bound anywhere: skipping
			// it would drop the predicate silently and return every related
			// record, answering a wider question in the shape of the right
			// answer. So a hop radius refuses, loudly, until the binding covers
			// it — see the note at the top of querygeo.go.
			if !root {
				refusals = append(refusals, apperrors.FieldRefusal{
					Field: at + ".op", Code: CodeDistanceRankingUnavailable,
					Message: "a radius inside a traversal is not answered yet; " +
						"ask for it on the record you are searching for instead",
				})
			}
			continue
		}
		field, expr, refusal := c.resolve(alias, columns, vocab, at, clause)
		if refusal != nil {
			refusals = append(refusals, *refusal)
			continue
		}
		fragment, refusal := c.clause(expr, at, field, clause)
		if refusal != nil {
			refusals = append(refusals, *refusal)
			continue
		}
		// The guard rides the predicate rather than the statement's where-list
		// so a traversal's own clauses carry it too — predicates renders both.
		guard, err := referenceGuard(ctx, vocab.Target, field, expr, refAlias(c.refs), c.arg)
		if err != nil {
			return nil, nil, err
		}
		if guard != "" {
			c.refs++
			fragment = "(" + fragment + ") AND " + guard
		}
		fragments = append(fragments, fragment)
	}
	return fragments, refusals, nil
}

// resolve reads the field a clause names and the expression it compiles to,
// refusing a name this workspace cannot answer.
//
// Both refusals are unreachable through Execute: the validator settled
// membership against this same vocabulary, and a published field compiles.
// Reaching either means the executor was handed a plan that never passed
// validation, which is a wiring fault to fail loudly on rather than a caller to
// explain it to.
func (c *planCompiler) resolve(
	alias string, columns *storage, vocab TargetVocabulary, at string, clause Predicate,
) (Field, string, *apperrors.FieldRefusal) {
	field, ok := vocab.Field(clause.Field)
	if !ok {
		return Field{}, "", &apperrors.FieldRefusal{
			Field: at + ".field", Code: CodeUnknownField,
			Message: "the query plan cannot name " + quote(clause.Field) + " on " + quote(vocab.Target),
		}
	}
	expr, ok := columns.expr(alias, field)
	if !ok {
		return Field{}, "", &apperrors.FieldRefusal{
			Field: at + ".field", Code: CodeUnknownField,
			Message: quote(clause.Field) + " cannot be answered by this workspace's records",
		}
	}
	return field, expr, nil
}

// clause renders one `field op value`.
func (c *planCompiler) clause(expr, at string, field Field, clause Predicate) (string, *apperrors.FieldRefusal) {
	if clause.Op == OpIn {
		return c.inClause(expr, at, field, clause)
	}
	value, cast, refusal := c.bind(at+"."+memberValue, field, clause.Value)
	if refusal != nil {
		return "", refusal
	}
	operand := fmt.Sprintf("$%d%s", c.arg(value), cast)
	if clause.Op == OpNeq {
		// IS DISTINCT FROM rather than <>: a field that is UNSET is distinct
		// from every value, and three-valued logic would otherwise drop those
		// rows from an answer the caller reads as "everything that is not X".
		return expr + " IS DISTINCT FROM " + operand, nil
	}
	comparator, ok := sqlComparators[clause.Op]
	if !ok {
		return "", &apperrors.FieldRefusal{
			Field: at + ".op", Code: CodeUnknownOperator,
			Message: quote(clause.Op) + " is not an operator this workspace can answer",
		}
	}
	return expr + " " + comparator + " " + operand, nil
}

// inClause renders a membership test as an explicit parameter list. Each
// element binds under the field's own kind, so a list carrying one operand of
// the wrong shape is refused naming that element rather than the clause.
func (c *planCompiler) inClause(expr, at string, field Field, clause Predicate) (string, *apperrors.FieldRefusal) {
	var values []json.RawMessage
	if err := json.Unmarshal(clause.Values, &values); err != nil {
		return "", &apperrors.FieldRefusal{
			Field: at + "." + memberValues, Code: CodeValueMissing,
			Message: quote(OpIn) + " needs a non-empty " + quote(memberValues) + " list",
		}
	}
	operands := make([]string, len(values))
	for i, raw := range values {
		value, cast, refusal := c.bind(at+"."+memberValues+"["+strconv.Itoa(i)+"]", field, raw)
		if refusal != nil {
			return "", refusal
		}
		operands[i] = fmt.Sprintf("$%d%s", c.arg(value), cast)
	}
	return expr + " IN (" + strings.Join(operands, ", ") + ")", nil
}

// sqlComparators maps the ordered and equality operators onto their SQL
// spelling. `neq` and `in` are absent deliberately — both need more than a
// comparator, and putting a wrong one here would be silent.
var sqlComparators = map[string]string{
	OpEq:  "=",
	OpLt:  "<",
	OpLte: "<=",
	OpGt:  ">",
	OpGte: ">=",
}

// dateLayout is the contract's date encoding. An instant carries the contract's
// own timestamp encoding instead (time.RFC3339, at the one call site that binds
// one), so there is nothing to name twice.
const dateLayout = "2006-01-02"

// idsIn renders a membership test over already-resolved ids. They are this
// server's own, so the list is bound rather than rendered, and it exists only
// to narrow the statement to what the similarity lane ranked.
func (c *planCompiler) idsIn(expr string, values []ids.UUID) string {
	operands := make([]string, len(values))
	for i, id := range values {
		operands[i] = "$" + strconv.Itoa(c.arg(id))
	}
	return expr + " IN (" + strings.Join(operands, ", ") + ")"
}
