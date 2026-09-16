// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A LIST THAT CAN BE CUT SAYS WHERE IT WAS CUT.
//
// A route declaring `limit` and nothing else answers the first page and says
// nothing about the rest. That is not a slow answer — it is a complete-looking
// one. The rows past the ceiling are unreachable through the route at all, for
// every caller, however patient, and the screen above it draws a finished list
// with no error, no count and nothing to press.
//
// It has shipped twice: the disclosure-duty queue, where an installation owing
// 1,285 duties could reach 200 of them (#5511), and the confirm-submissions
// queue, where the submissions that fell off the end were the newest and the
// resolved archive was cut permanently (#5707). Two of the same defect is what
// says a point fix under-counts, so the obligation is derived from the contract
// instead: every operation that declares `limit` either declares a cursor and
// answers a `page`, or is named below with the reason it cannot be cut short.
//
// Derived from crm.yaml itself, so a route added tomorrow joins the census by
// existing rather than by somebody remembering it.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// boundedButWhole are the operations that take a `limit` and are still a whole
// answer, with the reason each is.
//
// A register rather than a skip: an entry here is a claim somebody wrote down
// and the next reader can check. It only shrinks — a route that grows a
// continuation leaves, and one that cannot be cut short says why it cannot.
var boundedButWhole = gatekit.Waive(map[string]string{
	"explainLeadScore (/leads/{id}/score)": "the bound applies to the retained series, which " +
		"the route answers only under `history=true` — it takes a cursor and carries a `page` " +
		"for that arm, and the required set is the CURRENT explanation's, where there is one " +
		"score and nothing to continue",
	"getMyLinkedInReach (/me/linkedin-reach)": "ranked by connection count and bounded, and the " +
		"answer carries `accounts_total` — every account reached rather than the page returned — " +
		"so a reader is never handed a truncated list as the whole network. The tail is still " +
		"unreachable; issue 5730",
	"getMagic (/magic)": "a WINDOW rather than a queue: the read reports from `since`, defaulting " +
		"to the reader's last brief cutoff, so its bound cuts a period a reader has already been " +
		"shown rather than a backlog that accumulates. The contract says a cursor arrives with " +
		"the lane that needs one, which is the sentence this gate exists to doubt; issue 5730",
	"listCommunicationReviews (/communication-reviews)": "a reviewer's queue with the defect this " +
		"gate was written for, unfixed: oldest first, bounded at 100, no cursor. Fixing it here " +
		"would multiply a paging change across a module this one does not touch; issue 5730",
	"listWebhookDeliveries (/webhook-subscriptions/{id}/deliveries)": "answers a `page` and takes " +
		"no cursor, so a caller is told there is more and handed no way to ask for it — the same " +
		"defect, half-built; issue 5730",
})

// listPagingHolds is the census.
func TestEveryBoundedListSaysWhereItWasCut(t *testing.T) {
	t.Parallel()
	doc := pagedListContract(t)
	ops := boundedListOperations(t, doc)
	// A CENSUS THAT READ NOTHING reports the same silence as one that found
	// nothing wrong. This tree publishes dozens of bounded lists; a walk that
	// found a handful has stopped reading the contract rather than stopped
	// finding offenders.
	if len(ops) < 40 {
		t.Fatalf("the walk found %d bounded list operations, and the contract publishes "+
			"many more: this gate has stopped reading crm.yaml rather than stopped finding "+
			"offenders", len(ops))
	}
	for _, op := range ops {
		if op.cursor && op.page {
			continue
		}
		if boundedButWhole.Waived(t, op.id) {
			continue
		}
		t.Errorf("%s declares `limit` and %s: a caller that reaches the ceiling has no way to "+
			"ask for the rest, and the screen above it draws a finished list. Give it a cursor "+
			"and a `page`, or name it in boundedButWhole with the reason it cannot be cut short",
			op.id, op.missing())
	}
	boundedButWhole.AssertAllMatched(t)
}

// boundedList is one operation that takes a `limit`, and what it offers a
// caller who reaches it.
type boundedList struct {
	id     string
	cursor bool
	page   bool
}

func (b boundedList) missing() string {
	switch {
	case !b.cursor && !b.page:
		return "neither a cursor nor a continuation in its answer"
	case !b.cursor:
		return "no cursor to continue with"
	default:
		return "no continuation in its answer saying whether there is more"
	}
}

// boundedListOperations reads every GET that declares a `limit`, and asks the
// two questions of each: does it take a cursor, and does its 200 answer a
// `page`.
//
// The `limit` is recognised through the shared parameter ref as well as by
// name: most routes reach for `#/components/parameters/Limit`, and one that
// spells its own bound out is bounded just the same.
func boundedListOperations(t *testing.T, doc pagedListSpec) []boundedList {
	t.Helper()
	var out []boundedList
	for path, item := range doc.Paths {
		for method, op := range item {
			if !strings.EqualFold(method, "get") || op.OperationID == "" {
				continue
			}
			bounded, cursor := false, false
			for _, p := range append(append([]pagedListParam{}, item.parameters()...), op.Parameters...) {
				switch {
				case p.Name == "limit" || strings.HasSuffix(p.Ref, "/Limit"):
					bounded = true
				case p.Name == "cursor" || strings.HasSuffix(p.Ref, "/Cursor"):
					cursor = true
				}
			}
			if !bounded {
				continue
			}
			out = append(out, boundedList{
				id:     op.OperationID + " (" + path + ")",
				cursor: cursor,
				page:   op.answersAContinuation(doc.Components.Schemas),
			})
		}
	}
	return out
}

// ─── The contract, read only as far as this question needs ─────────────────

type pagedListSpec struct {
	Paths      map[string]pagedListPathItem `yaml:"paths"`
	Components struct {
		Schemas map[string]map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

// pagedListPathItem is a path's operations plus the parameters every operation
// under it inherits. `parameters` is a sibling of the methods rather than one of
// them, so it is read out of the same map and then removed from the walk.
type pagedListPathItem map[string]pagedListOperation

func (i pagedListPathItem) parameters() []pagedListParam {
	return i["parameters"].Shared
}

type pagedListOperation struct {
	//nolint:tagliatelle // the key is the contract's own, and OpenAPI spells it in camel
	OperationID string           `yaml:"operationId"`
	Parameters  []pagedListParam `yaml:"parameters"`
	Responses   map[string]any   `yaml:"responses"`
	Shared      []pagedListParam `yaml:"-"`
}

// UnmarshalYAML reads either shape a key under a path can hold: an operation
// object, or the bare `parameters` sequence the path shares with all of them.
func (o *pagedListOperation) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		return node.Decode(&o.Shared)
	}
	type plain pagedListOperation
	var raw plain
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*o = pagedListOperation(raw)
	return nil
}

type pagedListParam struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"$ref"`
}

// answersAContinuation reports whether the 200 answer tells a caller there is
// more and how to ask for it.
//
// TWO SPELLINGS, because the tree has two. Most lists carry the shared `page`
// envelope, and it has to be REQUIRED rather than merely present: a
// continuation a client cannot rely on is one it has to branch around, and a
// list whose `page` is optional is a list that sometimes forgets to say it was
// cut. The ranked worklist spells its continuation flat instead, as a
// `next_cursor` beside its own readings, and a gate that insisted on the
// envelope would be asking that surface to restate its shape for the gate's
// convenience rather than for a caller's.
//
// A response schema is as often a `$ref` to a named envelope as it is inline —
// `ContactListResponse` and its two dozen siblings — so the ref is followed.
// Reading only the inline shape reported thirty paged routes as unpaged, which
// is the noise that gets a gate switched off.
func (o pagedListOperation) answersAContinuation(schemas map[string]map[string]any) bool {
	ok, found := o.Responses["200"].(map[string]any)
	if !found {
		return false
	}
	content, found := ok["content"].(map[string]any)
	if !found {
		return false
	}
	body, found := content["application/json"].(map[string]any)
	if !found {
		return false
	}
	schema, found := body["schema"].(map[string]any)
	if !found {
		return false
	}
	if ref, isRef := schema["$ref"].(string); isRef {
		named, known := schemas[ref[strings.LastIndex(ref, "/")+1:]]
		if !known {
			return false
		}
		schema = named
	}
	if properties, found := schema["properties"].(map[string]any); found {
		if _, flat := properties["next_cursor"]; flat {
			return true
		}
	}
	required, found := schema["required"].([]any)
	if !found {
		return false
	}
	for _, name := range required {
		if text, isText := name.(string); isText && text == "page" {
			return true
		}
	}
	return false
}

func pagedListContract(t *testing.T) pagedListSpec {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc pagedListSpec
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("the contract parsed to no paths at all, so this gate read nothing")
	}
	return doc
}
