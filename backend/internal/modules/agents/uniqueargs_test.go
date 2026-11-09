// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// merge_records advertises member_ids as uniqueItems, as its REST twin does, so
// the real tool refuses a member named twice before anything runs. One spelling
// in upper case names the same contact.
func TestMergeRecordsRefusesACaseMemberNamedTwice(t *testing.T) {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterCoreTools(registry, seamProbeProvider{}, nil, nil, nil, nil, nil)
	member, survivor := ids.NewV7(), ids.NewV7()
	for name, repeat := range map[string]string{
		"the same spelling": member.String(),
		"another case":      strings.ToUpper(member.String()),
	} {
		t.Run(name, func(t *testing.T) {
			args := `{"record_type":"contact","contact_id":"` + member.String() + `","member_ids":["` +
				member.String() + `","` + survivor.String() + `","` + repeat + `"],"survivor_id":"` +
				survivor.String() + `","owner_id":"` + survivor.String() + `","revision":"r1"}`
			_, err := registry.Invoke(scopedAgentCtx(principal.ScopeWrite), "merge_records", json.RawMessage(args))
			var bad *BadArgsError
			if !errors.As(err, &bad) || bad.Field != "member_ids" || bad.Code != "duplicate" || !strings.Contains(bad.Error(), "item [2] repeats item [0]") {
				t.Fatalf("merge_records with a repeated member → %v, want a refusal naming member_ids", err)
			}
		})
	}
}

func TestAUniqueListArgumentComparesItemsAsJSONValues(t *testing.T) {
	tool := &fakeTool{spec: mcp.ToolSpec{
		Name: "unique_list", Title: "Unique list", Version: testToolVersion, Description: describedForRegistration,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"links":{"type":"array","uniqueItems":true,"items":{"type":"object"}}},"additionalProperties":false}`),
	}}
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(tool)
	ctx := scopedAgentCtx(principal.ScopeRead)
	if _, err := registry.Invoke(ctx, "unique_list", json.RawMessage(`{"links":[{"a":1,"b":2},{"b":2, "a":1}]}`)); err == nil {
		t.Fatal("accepted one object spelled twice")
	}
	if tool.handled {
		t.Fatal("the tool ran on a repeated item")
	}
	if _, err := registry.Invoke(ctx, "unique_list", json.RawMessage(`{"links":[{"a":1},{"a":2}]}`)); err != nil {
		t.Fatalf("refused two distinct items: %v", err)
	}
}

func TestItemsOfDifferentJSONValuesAreNotARepeat(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items string
	}{
		{"null and the empty string", `[null,""]`},
		{"a number and its digits as text", `[1,"1"]`},
		{"true and its word as text", `[true,"true"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry, ctx := uniqueListRegistry()
			if _, err := registry.Invoke(ctx, "unique_list", json.RawMessage(`{"links":`+tc.items+`}`)); err != nil {
				t.Fatalf("refused distinct items: %v", err)
			}
		})
	}
}

func TestAnItemKeyKeepsAnIntegerAFloatWouldRound(t *testing.T) {
	if wireItemKey(json.RawMessage(`9007199254740992`)) == wireItemKey(json.RawMessage(`9007199254740993`)) {
		t.Fatal("two distinct integers share a key")
	}
}

// A call that breaks a range and repeats a list item learns both in one answer.
func TestARepeatedItemAnswersBesideABrokenRange(t *testing.T) {
	registry, ctx := uniqueListRegistry()
	_, err := registry.Invoke(ctx, "unique_list", json.RawMessage(`{"links":["a","a"],"limit":0}`))
	var bad *BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Error(), "limit") || !strings.Contains(bad.Error(), "repeats item") {
		t.Fatalf("Invoke → %v, want one refusal naming the range and the repeat", err)
	}
}

func uniqueListRegistry() (*Registry, context.Context) {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(&fakeTool{spec: mcp.ToolSpec{
		Name: "unique_list", Title: "Unique list", Version: testToolVersion, Description: describedForRegistration,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"links":{"type":"array","uniqueItems":true},
			"limit":{"type":"integer","minimum":1}},"additionalProperties":false}`),
	}})
	return registry, scopedAgentCtx(principal.ScopeRead)
}
