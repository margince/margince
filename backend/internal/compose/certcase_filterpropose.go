// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The plain-words filter's certification case.
//
// What this site must get right is the TREE a reader is then shown, after the
// gate has dropped what the vocabulary cannot express, and the honesty of what
// it names back. So the expected value is that tree plus how many phrases must
// be named as unusable, and the comparison reads the tree for what it selects
// rather than how it is spelled: `in [DE, AT]` and `DE or AT` are one filter.
//
// Run calls filterpropose.Request and Evaluate calls filterpropose.Parse and
// filterpropose.Gate, so a change to either is a change to what is certified.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/filterpropose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const filterProposeSite = "nl_search/filter_propose"

// The words the canonical rendering is spelled in.
const (
	canonicalAnd  = "and"
	canonicalOr   = "or"
	canonicalNone = "none"
)

// filterProposeFixture is one proposal's input as the endpoint assembles it.
type filterProposeFixture struct {
	Resource string                `json:"resource"`
	Text     string                `json:"text"`
	Today    string                `json:"today"`
	Lang     string                `json:"lang"`
	Fields   []filterpropose.Field `json:"fields"`
}

// filterProposeExpectation is the tree that should survive the gate, or null,
// and the least number of phrases that must be named as unusable. Zero is
// exact: a sentence every word of which the vocabulary expresses has nothing
// to name back.
type filterProposeExpectation struct {
	Filter      *storekit.Predicate `json:"filter"`
	Unsupported int                 `json:"unsupported"`
}

type filterProposeCases struct{}

func (filterProposeCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskNlSearch, Variant: "filter_propose", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no model answer could pass: an expected clause
// naming a field or operator the fixture's vocabulary does not offer is one the
// gate drops whatever the model says.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (filterProposeCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var in filterProposeFixture
	if err := json.Unmarshal(fixture, &in); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", filterProposeSite, err)
	}
	today, err := time.Parse(time.DateOnly, in.Today)
	if err != nil {
		return nil, fmt.Errorf("%s: today must be a YYYY-MM-DD date, the one the endpoint states: %w", filterProposeSite, err)
	}
	if strings.TrimSpace(in.Text) == "" || in.Resource == "" || len(in.Fields) == 0 {
		return nil, fmt.Errorf("%s: the fixture needs a resource, a sentence and a vocabulary — the endpoint refuses any of them missing", filterProposeSite)
	}
	var want filterProposeExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {filter, unsupported}: %w", filterProposeSite, err)
	}
	if want.Filter != nil {
		if err := offeredBy(*want.Filter, in.Fields); err != nil {
			return nil, fmt.Errorf("%s: %w", filterProposeSite, err)
		}
	}
	lang := in.Lang
	if lang == "" {
		lang = "en"
	}
	return &filterProposeCase{
		in:       filterpropose.Input{Resource: in.Resource, Text: in.Text, Fields: in.Fields, Today: today, Lang: lang},
		expected: want,
	}, nil
}

// offeredBy answers why an expected tree could never survive the gate, or nil.
func offeredBy(p storekit.Predicate, fields []filterpropose.Field) error {
	for _, child := range append(append([]storekit.Predicate{}, p.And...), p.Or...) {
		if err := offeredBy(child, fields); err != nil {
			return err
		}
	}
	if p.Field == "" {
		return nil
	}
	for _, field := range fields {
		if field.Name != p.Field {
			continue
		}
		for _, op := range field.Operators {
			if op == p.Op {
				return nil
			}
		}
		return fmt.Errorf("the expected clause on %q uses %q, which the fixture's vocabulary does not offer", p.Field, p.Op)
	}
	return fmt.Errorf("the expected clause names %q, which the fixture's vocabulary does not offer", p.Field)
}

type filterProposeCase struct {
	in       filterpropose.Input
	expected filterProposeExpectation
}

func (c *filterProposeCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := filterpropose.Request(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", filterProposeSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production parser and gate and asks
// whether what survived selects what the scenario says it should.
func (c *filterProposeCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	answer, err := filterpropose.Parse(trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	proposal := filterpropose.Gate(answer, c.in.Fields)
	got, want := canonicalTree(proposal.Tree), canonicalTree(c.expected.Filter)
	if got != want {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the surviving filter is %s, wanted %s", got, want),
		}
	}
	named := len(proposal.Unsupported)
	if (c.expected.Unsupported == 0 && named != 0) || named < c.expected.Unsupported {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("%d phrases were named as unusable, wanted %s", named, atLeast(c.expected.Unsupported)),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func atLeast(n int) string {
	if n == 0 {
		return canonicalNone
	}
	return fmt.Sprintf("at least %d", n)
}

// canonicalTree renders a tree by what it selects: groups of one collapse,
// nested groups of the same join flatten, equalities on one field under an
// `or` become one `in`, text compares case-insensitively and siblings sort.
func canonicalTree(p *storekit.Predicate) string {
	if p == nil {
		return canonicalNone
	}
	return canonicalNode(*p)
}

func canonicalNode(p storekit.Predicate) string {
	join, children := canonicalAnd, p.And
	if len(p.Or) > 0 {
		join, children = canonicalOr, p.Or
	}
	if len(children) == 0 {
		return canonicalLeaf(p.Field, p.Op, p.Value)
	}
	if len(children) == 1 {
		return canonicalNode(children[0])
	}
	var parts []string
	membership := map[string]map[string]bool{}
	for _, child := range flatten(join, children) {
		if join == canonicalOr && isMembership(child) {
			if membership[child.Field] == nil {
				membership[child.Field] = map[string]bool{}
			}
			for _, value := range membersOf(child.Value) {
				membership[child.Field][value] = true
			}
			continue
		}
		parts = append(parts, canonicalNode(child))
	}
	for field, set := range membership {
		parts = append(parts, membershipLeaf(field, set))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	sort.Strings(parts)
	return join + "(" + strings.Join(parts, "; ") + ")"
}

// isMembership says a leaf asks "is the field one of these": an equality or an
// `in`, which an or-group unions into one set per field.
func isMembership(p storekit.Predicate) bool {
	return p.Field != "" && (p.Op == storekit.OpEq || p.Op == storekit.OpIn)
}

// membersOf is a membership leaf's values in canonical spelling.
//
//craft:ignore naked-any value is a predicate leaf's operand, a JSON scalar or list by the engine's own contract
func membersOf(value any) []string {
	list, ok := value.([]any)
	if !ok {
		return []string{canonicalValue(value)}
	}
	out := make([]string, 0, len(list))
	for _, member := range list {
		out = append(out, canonicalValue(member))
	}
	return out
}

// membershipLeaf renders one field's value set: a single value is an equality,
// several are an `in`, deduplicated and sorted.
func membershipLeaf(field string, set map[string]bool) string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	if len(values) == 1 {
		return fmt.Sprintf("%s eq %s", field, values[0])
	}
	return fmt.Sprintf("%s in [%s]", field, strings.Join(values, ","))
}

// flatten lifts a child group joined the same way as its parent into the
// parent: and(a, and(b, c)) selects what and(a, b, c) does.
func flatten(join string, children []storekit.Predicate) []storekit.Predicate {
	var out []storekit.Predicate
	for _, child := range children {
		switch {
		case join == canonicalAnd && len(child.And) > 0:
			out = append(out, flatten(join, child.And)...)
		case join == canonicalOr && len(child.Or) > 0:
			out = append(out, flatten(join, child.Or)...)
		default:
			out = append(out, child)
		}
	}
	return out
}

//craft:ignore naked-any value is a predicate leaf's operand, a JSON scalar, list or relative date by the engine's own contract
func canonicalLeaf(field, op string, value any) string {
	if op == storekit.OpIn {
		set := map[string]bool{}
		for _, member := range membersOf(value) {
			set[member] = true
		}
		return membershipLeaf(field, set)
	}
	return fmt.Sprintf("%s %s %s", field, op, canonicalValue(value))
}

//craft:ignore naked-any value is a predicate leaf's operand, a JSON scalar or relative date by the engine's own contract
func canonicalValue(value any) string {
	if relative, ok := value.(map[string]any); ok {
		return fmt.Sprintf("days_ago(%v)", relative["days_ago"])
	}
	if text, ok := value.(string); ok {
		return strings.ToLower(strings.TrimSpace(text))
	}
	return fmt.Sprintf("%v", value)
}
