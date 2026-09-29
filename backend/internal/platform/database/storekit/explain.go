// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Why one record is, or is not, selected by a filter.
//
// The explanation is built from the SAME leaf expressions CompilePredicate
// renders, evaluated for one row in one statement. The statement also carries
// the whole compiled filter, and the explanation is refused if the two answers
// differ, so a reader is never shown a reason that disagrees with the
// membership they are looking at.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExplainNode is one node of an explained filter: a group with its children,
// or a leaf with its verdict. Result is nil where SQL answers unknown, which a
// filter treats as not selected.
type ExplainNode struct {
	Join     string        `json:"join,omitempty"`
	Children []ExplainNode `json:"children,omitempty"`
	Field    string        `json:"field,omitempty"`
	Op       string        `json:"op,omitempty"`
	Operand  any           `json:"operand,omitempty"`
	Result   *bool         `json:"result"`
	// Value is the record's current value of a leaf's field as text, nil when
	// it holds none. Hidden says the value is not shown to this reader: a
	// masked column, a withheld field, or a fact on a linked record.
	Value  *string `json:"value,omitempty"`
	Hidden bool    `json:"hidden,omitempty"`
}

// Explanation is the verdict for one record: whether the filter selects it,
// whether it passed the resource's own base clause (a live, eligible row),
// and the explained tree.
type Explanation struct {
	Selected   bool
	PassesBase bool
	Root       ExplainNode
}

// ErrExplanationDisagrees is an explained tree whose combined verdict differs
// from the compiled filter's for the same row: a defect in this file, surfaced
// rather than shown.
var ErrExplanationDisagrees = errors.New("storekit: the explanation disagrees with the filter")

// explainLeaf is one leaf's rendered verdict and value columns.
type explainLeaf struct {
	node     *ExplainNode
	verdict  string
	value    string
	hasValue bool
}

// Explain evaluates the filter for one record the caller can see. A record
// outside the caller's row scope, or absent, answers ErrNotFound.
func (q Query) Explain(ctx context.Context, tx pgx.Tx, p Predicate, recordID ids.UUID) (Explanation, error) {
	if err := auth.Require(ctx, q.Table, principal.ActionRead); err != nil {
		return Explanation{}, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	compiled, err := CompilePredicate(p, q.Fields, arg)
	if err != nil {
		return Explanation{}, err
	}
	root, leaves, err := q.explainTree(ctx, p, arg)
	if err != nil {
		return Explanation{}, err
	}
	base := "TRUE"
	if q.BaseWhere != "" {
		base = q.BaseWhere
	}
	selects := []string{"(" + base + ")", "(" + compiled + ")"}
	for _, leaf := range leaves {
		selects = append(selects, "("+leaf.verdict+")")
		if leaf.hasValue {
			selects = append(selects, "("+leaf.value+")::text")
		}
	}
	where := fmt.Sprintf("t.id = $%d", arg(recordID))
	scope, err := auth.ScopeClauseFor(ctx, q.Table, "t", arg)
	if err != nil {
		return Explanation{}, err
	}
	if scope != "" {
		where += " AND " + scope
	}
	sql := fmt.Sprintf("SELECT %s FROM %s t WHERE %s", strings.Join(selects, ", "), q.Table, where)
	return scanExplanation(tx.QueryRow(ctx, sql, args...), root, leaves)
}

// scanExplanation reads the one explained row into the tree and checks the
// combined verdict against the compiled filter's.
func scanExplanation(row pgx.Row, root *ExplainNode, leaves []explainLeaf) (Explanation, error) {
	var passesBase bool
	var filterSelects *bool
	targets := []any{&passesBase, &filterSelects}
	for _, leaf := range leaves {
		targets = append(targets, &leaf.node.Result)
		if leaf.hasValue {
			targets = append(targets, &leaf.node.Value)
		}
	}
	if err := row.Scan(targets...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Explanation{}, apperrors.ErrNotFound
		}
		return Explanation{}, fmt.Errorf("explain a filter for one record: %w", err)
	}
	combine(root)
	if isTrue(root.Result) != isTrue(filterSelects) {
		return Explanation{}, fmt.Errorf("%w: the tree says %v, the filter %v",
			ErrExplanationDisagrees, isTrue(root.Result), isTrue(filterSelects))
	}
	return Explanation{Selected: passesBase && isTrue(filterSelects), PassesBase: passesBase, Root: *root}, nil
}

// explainTree mirrors the filter as a tree of nodes and renders each leaf's
// verdict through compileLeaf, the function CompilePredicate uses, so a leaf
// is judged by exactly the SQL that decides membership.
func (q Query) explainTree(ctx context.Context, p Predicate, arg func(any) int) (*ExplainNode, []explainLeaf, error) {
	var leaves []explainLeaf
	counted := 0
	var walk func(p Predicate, node *ExplainNode) error
	walk = func(p Predicate, node *ExplainNode) error {
		join, children, isGroup, err := groupShape(p)
		if err != nil {
			return err
		}
		if isGroup {
			node.Join = join
			node.Children = make([]ExplainNode, len(children))
			for i, child := range children {
				if err := walk(child, &node.Children[i]); err != nil {
					return err
				}
			}
			return nil
		}
		verdict, err := compileLeaf(p, q.Fields, arg, &counted)
		if err != nil {
			return err
		}
		node.Field, node.Op, node.Operand = p.Field, p.Op, p.Value
		leaf := explainLeaf{node: node, verdict: verdict}
		field := q.Fields[p.Field]
		if field.Withheld || field.Link != "" {
			node.Hidden = true
		} else {
			value, masked, err := q.shownValue(ctx, p.Field, field, arg)
			if err != nil {
				return err
			}
			node.Hidden = masked
			leaf.value, leaf.hasValue = value, !masked
		}
		leaves = append(leaves, leaf)
		return nil
	}
	root := &ExplainNode{}
	if err := walk(p, root); err != nil {
		return nil, nil, err
	}
	return root, leaves, nil
}

// shownValue is the leaf field's value as the reader may see it. A field any
// mask of this reader's covers is not shown at all, rather than shown on the
// rows the mask spares: a why that showed a value on some members and not
// others would tell the reader which rows the mask spares.
func (q Query) shownValue(ctx context.Context, name string, field Field, arg func(any) int) (string, bool, error) {
	_, masked, err := auth.MaskExcludedClause(ctx, q.Table, name, "t", arg)
	if err != nil {
		return "", false, err
	}
	return field.Expr, masked, nil
}

// combine folds each group's children into the group's verdict with SQL's
// three-valued AND and OR, so an unknown leaf is unknown here as it is there.
func combine(node *ExplainNode) {
	if node.Join == "" {
		return
	}
	var result *bool
	for i := range node.Children {
		combine(&node.Children[i])
		child := node.Children[i].Result
		if i == 0 {
			result = child
			continue
		}
		if node.Join == "or" {
			result = or3(result, child)
		} else {
			result = and3(result, child)
		}
	}
	node.Result = result
}

func and3(a, b *bool) *bool {
	switch {
	case isFalse(a) || isFalse(b):
		return boolPtr(false)
	case a == nil || b == nil:
		return nil
	default:
		return boolPtr(true)
	}
}

func or3(a, b *bool) *bool {
	switch {
	case isTrue(a) || isTrue(b):
		return boolPtr(true)
	case a == nil || b == nil:
		return nil
	default:
		return boolPtr(false)
	}
}

func isTrue(b *bool) bool  { return b != nil && *b }
func isFalse(b *bool) bool { return b != nil && !*b }
func boolPtr(b bool) *bool { return &b }
