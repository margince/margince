// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filterpropose

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The reasons a phrase did not become a clause, as the contract names them.
const (
	CodeNotExpressible     = "not_expressible"
	CodeUnknownField       = "unknown_field"
	CodeOperatorNotAllowed = "operator_not_allowed"
	CodeValueNotAllowed    = "value_not_allowed"
	CodeTooManyConditions  = "too_many_conditions"
	CodeValueNotVerifiable = "value_not_verifiable"
)

// errOptionsWithheld answers a picklist value this caller cannot be shown the
// options of: accepting it unchecked would propose a filter that matches nothing
// and reads as a settled answer, so it is declined instead.
var errOptionsWithheld = errors.New("the options of this field are not visible to you, so the value cannot be checked")

// Dropped is one phrase the proposal cannot use, and why.
type Dropped struct {
	Phrase string
	Code   string
	Reason string
	// Field is the field a dropped clause named; empty for a phrase the model
	// itself reported as inexpressible.
	Field string
}

// Proposal is what survives Gate: a tree the engine accepts, or none, and every
// phrase that did not make it into one.
type Proposal struct {
	Tree        *storekit.Predicate
	Unsupported []Dropped
}

// Gate turns the model's answer into a tree the predicate engine accepts.
//
// Nothing the model says is trusted. Each clause must name a field the caller's
// vocabulary offers, with an operator that field lists, a value the real
// compiler binds and, for a picklist, one of its options. A clause failing any
// of those is dropped and named back rather than failing the proposal: the
// reader keeps every clause that was right and learns which phrase was not.
func Gate(answer Answer, fields []Field) Proposal {
	byName := make(map[string]Field, len(fields))
	engine := make(map[string]storekit.Field, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
		engine[field.Name] = storekit.Field{
			Expr: "t." + field.Name, Type: storekit.FieldType(field.Type),
			Options: field.Options, Currency: field.Currency,
		}
	}
	var out Proposal
	for _, reported := range answer.Unsupported {
		out.Unsupported = append(out.Unsupported, Dropped{
			Phrase: reported.Phrase, Code: CodeNotExpressible, Reason: reported.Reason,
		})
	}
	var groups []storekit.Predicate
	leaves := 0
	for _, group := range answer.Groups {
		var kept []storekit.Predicate
		for _, clause := range group.Clauses {
			leaf, dropped := admit(clause, byName, engine, &leaves)
			if dropped != nil {
				out.Unsupported = append(out.Unsupported, *dropped)
				continue
			}
			kept = append(kept, leaf)
		}
		if node, ok := joined(group.Join, kept); ok {
			groups = append(groups, node)
		}
	}
	out.Tree = rootOf(answer.Join, groups)
	return out
}

// admit checks one clause and answers the leaf it becomes, or why it cannot.
func admit(
	clause Clause, byName map[string]Field, engine map[string]storekit.Field, leaves *int,
) (storekit.Predicate, *Dropped) {
	drop := func(code, reason string) (storekit.Predicate, *Dropped) {
		phrase := clause.Phrase
		if strings.TrimSpace(phrase) == "" {
			phrase = clause.Field
		}
		return storekit.Predicate{}, &Dropped{Phrase: phrase, Code: code, Reason: reason, Field: clause.Field}
	}
	field, ok := byName[clause.Field]
	if !ok {
		return drop(CodeUnknownField, fmt.Sprintf("%q is not a field you can filter on here", clause.Field))
	}
	if !offers(field.Operators, clause.Op) {
		return drop(CodeOperatorNotAllowed, fmt.Sprintf("%q does not take the %q operator", field.Name, clause.Op))
	}
	value, err := operand(clause, field)
	if errors.Is(err, errOptionsWithheld) {
		return drop(CodeValueNotVerifiable, err.Error())
	}
	if err != nil {
		return drop(CodeValueNotAllowed, err.Error())
	}
	leaf := storekit.Predicate{Field: field.Name, Op: clause.Op, Value: value}
	// The real compiler, over this one leaf: what it refuses here the preview
	// and the saved list would refuse too.
	if _, err := storekit.CompilePredicate(leaf, engine, func(any) int { return 1 }); err != nil {
		var refused *storekit.PredicateError
		if errors.As(err, &refused) {
			return drop(CodeValueNotAllowed, refused.Message)
		}
		return drop(CodeValueNotAllowed, err.Error())
	}
	if *leaves >= storekit.PredicateMaxLeaves {
		return drop(CodeTooManyConditions,
			fmt.Sprintf("a filter holds at most %d conditions", storekit.PredicateMaxLeaves))
	}
	*leaves++
	return leaf, nil
}

func offers(ops []string, op string) bool {
	for _, offered := range ops {
		if offered == op {
			return true
		}
	}
	return false
}

// operand reads the clause's value slot the field's type and operator call for.
//
//craft:ignore naked-any the return is a predicate leaf's operand, a JSON scalar, list or relative date by the engine's own contract
func operand(clause Clause, field Field) (any, error) {
	switch {
	case clause.Op == storekit.OpExists:
		if clause.Flag == nil {
			return nil, errors.New("exists takes true or false")
		}
		return *clause.Flag, nil
	case clause.Op == storekit.OpIn:
		return listOperand(clause.List, field)
	case storekit.FieldType(field.Type) == storekit.FieldDate && clause.DaysAgo != nil:
		return relativeDay(clause, field)
	}
	switch storekit.FieldType(field.Type) {
	case storekit.FieldNumber:
		if clause.Number == nil {
			return nil, fmt.Errorf("%q takes a number", field.Name)
		}
		return *clause.Number, nil
	case storekit.FieldCurrency:
		if clause.Number == nil {
			return nil, fmt.Errorf("%q takes an amount", field.Name)
		}
		return minorUnits(*clause.Number, field)
	case storekit.FieldBoolean:
		if clause.Flag == nil {
			return nil, fmt.Errorf("%q takes true or false", field.Name)
		}
		return *clause.Flag, nil
	}
	if clause.Text == nil {
		return nil, fmt.Errorf("%q takes a value", field.Name)
	}
	return option(*clause.Text, field)
}

// relativeOrdering are the operators a relative date is proposed with: "in the
// last N days" and "more than N days ago" are bounds, never one exact day.
var relativeOrdering = map[string]bool{
	storekit.OpGt: true, storekit.OpGte: true, storekit.OpLt: true, storekit.OpLte: true,
}

// relativeDay is a days_ago operand, taken only as a bound and only counting
// back from today. Zero is today itself, which "since today" means.
//
//craft:ignore naked-any the return is a predicate leaf's operand, the engine's relative-date map
func relativeDay(clause Clause, field Field) (any, error) {
	if !relativeOrdering[clause.Op] {
		return nil, fmt.Errorf("a relative date on %q is a bound (before or since), not %q", field.Name, clause.Op)
	}
	if *clause.DaysAgo < 0 {
		return nil, fmt.Errorf("a relative date on %q counts back from today, so %d days is not one", field.Name, *clause.DaysAgo)
	}
	return map[string]any{"days_ago": float64(*clause.DaysAgo)}, nil
}

// listOperand is an `in` list, each member read the way a single value of the
// field's type is.
//
//craft:ignore naked-any the return is a predicate leaf's operand: []any of the field type's scalars
func listOperand(list []string, field Field) (any, error) {
	if len(list) == 0 {
		return nil, errors.New("in takes at least one value")
	}
	out := make([]any, 0, len(list))
	for _, member := range list {
		switch storekit.FieldType(field.Type) {
		case storekit.FieldNumber, storekit.FieldCurrency:
			number, err := strconv.ParseFloat(strings.TrimSpace(member), 64)
			if err != nil {
				return nil, fmt.Errorf("%q takes numbers, not %q", field.Name, member)
			}
			if storekit.FieldType(field.Type) == storekit.FieldNumber {
				out = append(out, number)
				continue
			}
			minor, err := minorUnits(number, field)
			if err != nil {
				return nil, err
			}
			out = append(out, minor)
		default:
			value, err := option(member, field)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
	}
	return out, nil
}

// option answers a value as the picklist spells it. The engine compiles a value
// outside the options and matches nothing, which a reader would take for a
// settled answer, so this is where such a value is refused instead.
//
// An exact match wins; a case-insensitive one is taken only when it names
// exactly one option, because options ["US", "us"] are two values.
func option(value string, field Field) (string, error) {
	kind := storekit.FieldType(field.Type)
	if kind != storekit.FieldPicklist && kind != storekit.FieldMultiselect {
		return value, nil
	}
	if len(field.Options) == 0 {
		return "", errOptionsWithheld
	}
	value = strings.TrimSpace(value)
	var folded []string
	for _, offered := range field.Options {
		if offered == value {
			return offered, nil
		}
		if strings.EqualFold(value, offered) {
			folded = append(folded, offered)
		}
	}
	if len(folded) == 1 {
		return folded[0], nil
	}
	return "", fmt.Errorf("%q is not one of the options of %q", value, field.Name)
}

// minorUnits scales an amount in major units into the minor units the engine
// compares, through the values package's per-currency digits.
func minorUnits(major float64, field Field) (float64, error) {
	if field.Currency == "" {
		return 0, fmt.Errorf("the currency of %q is not known, so an amount cannot be compared", field.Name)
	}
	minor, ok := values.MinorUnits(strconv.FormatFloat(major, 'f', -1, 64), field.Currency)
	if !ok {
		return 0, fmt.Errorf("%v is not an amount of %s", major, field.Currency)
	}
	return float64(minor), nil
}

// joined makes one group's surviving clauses a node: nothing, the clause
// itself, or a group.
func joined(join string, kept []storekit.Predicate) (storekit.Predicate, bool) {
	switch len(kept) {
	case 0:
		return storekit.Predicate{}, false
	case 1:
		return kept[0], true
	}
	if join == joinOr {
		return storekit.Predicate{Or: kept}, true
	}
	return storekit.Predicate{And: kept}, true
}

// rootOf answers the tree with a group at its root, because that is the shape
// the builder edits: a lone clause is wrapped, and a lone group is the root.
func rootOf(join string, groups []storekit.Predicate) *storekit.Predicate {
	switch {
	case len(groups) == 0:
		return nil
	case len(groups) == 1 && (len(groups[0].And) > 0 || len(groups[0].Or) > 0):
		return &groups[0]
	case len(groups) > 1 && join == joinOr:
		return &storekit.Predicate{Or: groups}
	default:
		return &storekit.Predicate{And: groups}
	}
}
