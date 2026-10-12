// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"maps"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestStagedSentenceIsTheResolversWhenItShowsEveryField(t *testing.T) {
	pol := agentPolicy{Op: "mergeTags", Tool: "merge_tags"}
	info := agents.StageInfo{Summary: `Fold tag "Conf" into "Conference 2026"`, TargetID: ids.NewV7()}
	body := []byte(`{"into_tag_id":"` + ids.NewV7().String() + `"}`)

	got := stagedSentence(info, inEnglish, pol, summaryRequest("POST", "/v1/tags/x/merge"), body)

	if !strings.HasPrefix(got, info.Summary) {
		t.Errorf("staged %q, want it to open with the resolver's %q", got, info.Summary)
	}
}

// A field the command erased still reaches the approver.
func TestStagedSentenceAppendsTheBodyFieldsTheResolverLeavesOut(t *testing.T) {
	pol := agentPolicy{Op: "sendEmail", Tool: "send_email"}
	info := agents.StageInfo{Summary: `Send an email to buyer@example.test, subject "Q3"`, TargetID: ids.NewV7()}
	body := []byte(`{"to":["buyer@example.test"],"subject":"Q3","bcc":["cfo@example.test"],"attachment_ids":["a"],"priority":2,"note":null}`)

	got := stagedSentence(info, inEnglish, pol, summaryRequest("POST", "/v1/emails"), body)

	for _, want := range []string{info.Summary + ": ", "bcc=[1]", "attachment_ids=[1]", "priority=2", "note=null"} {
		if !strings.Contains(got, want) {
			t.Errorf("staged %q, which lacks %q", got, want)
		}
	}
	for _, shown := range []string{"subject=", "to=["} {
		if strings.Contains(got, shown) {
			t.Errorf("staged %q repeats %q, which the sentence already shows", got, shown)
		}
	}
}

// A sentence that cannot be read, or that names a record by an id, is replaced
// by this door's own line.
func TestStagedSentenceFallsBackWhenTheResolverHasNoReadableLine(t *testing.T) {
	target := ids.NewV7()
	pol := agentPolicy{Op: "setDealStakeholder", Tool: "update_record", RecordType: recordTypeDeal}
	req := summaryRequest("PUT", "/v1/deals/"+target.String()+"/stakeholders/c")
	body := []byte(`{"role":"champion"}`)

	for name, info := range map[string]agents.StageInfo{
		"empty":           {TargetID: target},
		"the target":      {Summary: "Set the stakeholder on " + target.String(), TargetID: target},
		"another record":  {Summary: "Host " + ids.NewV7().String() + "'s calendar", TargetID: target},
		"an uppercase id": {Summary: "On " + strings.ToUpper(ids.NewV7().String()), TargetID: target},
	} {
		if got, want := stagedSentence(info, inEnglish, pol, req, body), restSummary(inEnglish, pol, req, body); got != want {
			t.Errorf("%s: staged %q, want this door's %q", name, got, want)
		}
	}
}

// What the approver reads must change when any field the diff_hash binds does.
// Two staged calls that differ in one bound field, and read the same, would let
// a human approve one and release the other.
func TestEveryRouteStagesADifferentSentenceWhenAnyBoundFieldDiffers(t *testing.T) {
	extra := map[string]map[string]any{
		"sendEmail":        {"bcc": []any{"cfo@example.test"}, "attachment_ids": []any{ids.NewV7().String()}},
		"sendCompanyEmail": {"bcc": []any{"cfo@example.test"}},
		"advanceDeal":      {"won_without_contract_reason": "other"},
	}
	pairs, staged := 0, map[string]bool{}
	for _, route := range sortedRoutes() {
		pol := agentPolicies[route]
		raw, written := contractBodies[pol.Op]
		if !written || pol.Access != accessTool || !mutatingMethod(strings.SplitN(route, " ", 2)[0]) {
			continue
		}
		var base map[string]any
		if json.Unmarshal([]byte(raw), &base) != nil || len(base) == 0 {
			continue
		}
		original, ok := stagedLine(route, pol, raw)
		if !ok {
			continue
		}
		staged[pol.Op] = true
		for _, variant := range boundFieldVariants(base, extra[pol.Op]) {
			changed, ok := stagedLine(route, pol, variant.body)
			if !ok {
				continue
			}
			pairs++
			if changed == original {
				t.Errorf("%s: changing %s stages the same sentence %q, so an approver cannot tell the calls apart",
					pol.Op, variant.what, original)
			}
		}
	}
	for op := range extra {
		if !staged[op] {
			t.Errorf("%s stages nothing here, so its erased fields are not held to the rule", op)
		}
	}
	if pairs < 35 {
		t.Fatalf("compared %d pairs of staged sentences, want at least 35", pairs)
	}
}

func sortedRoutes() []string {
	routes := make([]string, 0, len(agentPolicies))
	for route := range agentPolicies {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

// stagedLine is the summary the gate stages for one request, and false when
// the route cannot stage it with these dependencies.
func stagedLine(route string, pol agentPolicy, body string) (string, bool) {
	staging := &capturingApprovals{}
	req := syntheticOperandRequest(route, ids.NewV7())
	stageRefusal(httptest.NewRecorder(), req, staging, operandWalkDeps(), pol, []byte(body))
	return staging.last.Summary, staging.last.Summary != ""
}

type bodyVariant struct{ what, body string }

// boundFieldVariants changes each field of the body in turn, and adds the
// fields the route's command does not carry.
func boundFieldVariants(base, extra map[string]any) []bodyVariant {
	var out []bodyVariant
	keys := make([]string, 0, len(base))
	for key := range base {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		changed := cloneBody(base)
		changed[key] = otherValue(base[key])
		out = append(out, bodyVariant{key, stagedBodyJSON(changed)})
	}
	for key, value := range extra {
		added := cloneBody(base)
		added[key] = value
		out = append(out, bodyVariant{"the added " + key, stagedBodyJSON(added)})
	}
	return out
}

func cloneBody(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func stagedBodyJSON(body map[string]any) string {
	raw, _ := json.Marshal(body)
	return string(raw)
}

// otherValue is a different value of the same kind: a new id for an id, so the
// route still decodes it.
//
//craft:ignore naked-any a decoded JSON value is open by nature
func otherValue(value any) any {
	switch v := value.(type) {
	case string:
		if _, err := ids.Parse(v); err == nil {
			return ids.NewV7().String()
		}
		return v + " changed"
	case float64:
		return v + 1
	case bool:
		return !v
	case []any:
		if len(v) == 0 {
			return []any{"changed"}
		}
		return append(slices.Clone(v), v[len(v)-1])
	case map[string]any:
		out := cloneBody(v)
		out["changed"] = "yes"
		return out
	default:
		return "changed"
	}
}
