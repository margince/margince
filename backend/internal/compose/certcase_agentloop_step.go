// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The arguments a fixture's tool answer is matched on. An answer pins only the
// arguments that tell two calls of one tool apart, such as the record a
// read_record is about. A call carrying those gets that answer, whatever else it
// passes.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// agentLoopRegistry is the registered surface, resolved once: every tool a
// passport admitting every scope may call. The runner narrows it to what is
// offered and then to the agent's allowlist. It is built from the same
// NewRegistry every other sweep builds, with no pool. Registration is
// composition-time and needs no database, and a certification turn executes
// nothing.
var agentLoopRegistry = sync.OnceValue(func() []mcp.ToolSpec {
	return NewRegistry(nil, SendPath{}).Specs()
})

// agentLoopSchemaProperties names what a tool's advertised schema lets a caller
// pass. A schema with no properties object names nothing, which refuses every
// pinned argument — the right answer for a tool the window prints as taking none.
func agentLoopSchemaProperties(schema json.RawMessage) map[string]bool {
	var object struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &object); err != nil {
		return nil
	}
	declared := make(map[string]bool, len(object.Properties))
	for name := range object.Properties {
		declared[name] = true
	}
	return declared
}

// agentLoopArgDisagreements names every pinned argument the call did not carry
// as pinned. It is a SUBSET claim: an argument the fixture never named is not a
// disagreement, since a real call carries optional arguments nobody pinned.
func agentLoopArgDisagreements(want map[string]json.RawMessage, called json.RawMessage) []string {
	if len(want) == 0 {
		return nil
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(called, &got); err != nil {
		return []string{"the call carried no argument object to compare"}
	}
	out := make([]string, 0, len(want))
	for _, arg := range sortedArgNames(want) {
		actual, present := got[arg]
		if !present {
			out = append(out, fmt.Sprintf("%s was not passed", arg))
			continue
		}
		if !agentLoopSameJSON(want[arg], actual) {
			out = append(out, fmt.Sprintf("%s was passed as %s, not %s", arg, actual, want[arg]))
		}
	}
	return out
}

// agentLoopSameJSON compares two encoded values by what they mean, not how they
// were written. A fixture then neither fails on re-ordered keys nor passes on a
// different value that shares a prefix.
//
// Numbers are compared as exact rationals. Decoding into float64 would grade two
// integers past 2^53 equal. Comparing literals would read 5.0 and 5 as two values.
func agentLoopSameJSON(want, got json.RawMessage) bool {
	wantValue, err := agentLoopDecode(want)
	if err != nil {
		return false
	}
	gotValue, err := agentLoopDecode(got)
	if err != nil {
		return false
	}
	return agentLoopSameValue(wantValue, gotValue)
}

// agentLoopDecode reads one encoded value with numbers left as written, so the
// comparison below decides what they mean rather than inheriting float64's
// answer.
//
//craft:ignore naked-any a decoded JSON document is any by construction — the argument being compared is whatever the tool's schema admits, and encoding/json returns exactly this type.
func agentLoopDecode(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// agentLoopSameValue walks two decoded values together. It is a walk rather
// than reflect.DeepEqual because only the walk reaches the numbers: a pinned
// argument may be an object or a list, and a number nested in one is the same
// claim as a number that is one.
//
//craft:ignore naked-any it walks a decoded JSON document, whose nodes are the any that encoding/json produces — a narrower parameter would describe a shape the corpus is free not to use.
func agentLoopSameValue(want, got any) bool {
	switch wantValue := want.(type) {
	case json.Number:
		gotNumber, ok := got.(json.Number)
		return ok && agentLoopSameNumber(wantValue, gotNumber)
	case map[string]any:
		gotMap, ok := got.(map[string]any)
		if !ok || len(wantValue) != len(gotMap) {
			return false
		}
		for key, value := range wantValue {
			other, present := gotMap[key]
			if !present || !agentLoopSameValue(value, other) {
				return false
			}
		}
		return true
	case []any:
		gotList, ok := got.([]any)
		if !ok || len(wantValue) != len(gotList) {
			return false
		}
		for i, value := range wantValue {
			if !agentLoopSameValue(value, gotList[i]) {
				return false
			}
		}
		return true
	default:
		// Strings, booleans and null carry no representation to normalize, so
		// they mean what they are.
		return want == got
	}
}

// agentLoopSameNumber compares two JSON numbers exactly. A rational parses every
// literal JSON admits without rounding any of them, so 5 equals 5.0 and two
// integers past float64's reach stay apart.
func agentLoopSameNumber(want, got json.Number) bool {
	wantRat, wantOK := new(big.Rat).SetString(want.String())
	gotRat, gotOK := new(big.Rat).SetString(got.String())
	return wantOK && gotOK && wantRat.Cmp(gotRat) == 0
}

// sortedArgNames reads a map in one order, so a refusal names the same
// argument first however the fixture was written.
func sortedArgNames(args map[string]json.RawMessage) []string {
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
