// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The filler names a company wherever an enum argument picks the record a call
// acts on, and a fixture may name another kind. Each other word of that enum is
// a path the call takes for another kind of record, and its gates may ask other
// checks.
const filledKind = "company"

// isRecordKind reports whether word names a kind of record the filler hands out.
func isRecordKind(word string) bool {
	return word == filledKind || slices.Contains(slices.Collect(maps.Values(recordKinds)), word)
}

// kindIDArguments name the record the picked kind is about.
var kindIDArguments = []string{"id", "record_id", "entity_id"}

// kindVariants is one call per further record kind a call could name. The
// kinds are the enum of the argument the call fills with a record kind.
func kindVariants(t *testing.T, calls []toolRoleCall, seedable map[string]bool) []toolRoleCall {
	t.Helper()
	var out []toolRoleCall
	counting := countingPool(t)
	for _, call := range calls {
		filled, enums := call.args(counting), enumArguments(t, call)
		for _, name := range slices.Sorted(maps.Keys(enums)) {
			picked, named := filled[name].(string)
			if !named || !isRecordKind(picked) || !slices.Contains(enums[name], picked) {
				continue
			}
			for _, kind := range enums[name] {
				if kind == picked {
					continue
				}
				variant := call
				variant.row = call.row + "[" + name + "=" + kind + "]"
				variant.kindArg, variant.kind, variant.kindSeeded = name, kind, seedable[kind]
				out = append(out, variant)
			}
		}
	}
	return out
}

// enumArguments are a call's top-level arguments that admit only listed words.
func enumArguments(t *testing.T, call toolRoleCall) map[string][]string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []any `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(call.input, &schema); err != nil {
		t.Fatalf("%s's input schema does not read: %v", call.verb, err)
	}
	out := map[string][]string{}
	for name, prop := range schema.Properties {
		for _, word := range prop.Enum {
			if text, ok := word.(string); ok {
				out[name] = append(out[name], text)
			}
		}
	}
	return out
}

// withKind turns a call's arguments to its variant's kind, on a record of that
// kind where the sweep can seed one.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func (c toolRoleCall) withKind(args map[string]any, pool *recordPool) map[string]any {
	args[c.kindArg] = c.kind
	for _, name := range kindIDArguments {
		if _, named := args[name]; !named {
			continue
		}
		args[name] = placeholderUUID
		if c.kindSeeded {
			args[name] = pool.take(c.kind)
		}
	}
	return args
}

// seedableKinds are the record kinds the sweep knows how to make.
func seedableKinds(t *testing.T, e *apptest.AppEnv) map[string]bool {
	t.Helper()
	seedable := map[string]bool{}
	for kind := range toolRoleSeeders(t, e, "", &recordPool{fresh: map[string][]string{}, facts: map[string]map[string]string{}}) {
		seedable[kind] = true
	}
	for _, batch := range toolRoleBatchSeeders(t, e) {
		for _, kind := range batch.kinds {
			seedable[kind] = true
		}
	}
	return seedable
}

// measureEveryKind calls each kind variant as the admin. Each verb's row keeps
// the checks every path was asked. A path that stopped early was asked fewer,
// so the row only loses checks. tools/list then hides a verb only from a
// caller who lacks a check every kind needs.
func measureEveryKind(t *testing.T, e *apptest.AppEnv, recorded *toolCheckRecorder, measured toolChecksMeasure, variants []toolRoleCall) {
	t.Helper()
	if len(variants) == 0 {
		t.Fatal("no call picks its record by a kind argument, so no path but the company's was measured")
	}
	assignRoleTo(t, e, toolRoleUser, "admin")
	clients := passportClients(t, e, "admin kinds", 1+len(variants)/toolRoleCallsPerPassport)
	pool := seedToolRoleRecords(t, e, variants, "")
	for n, variant := range variants {
		client := clients[n/toolRoleCallsPerPassport]
		pool.caller = client.Bearer()
		recorded.take(variant.verb)
		got, _ := callThroughApproval(t, e, client, variant.verb, variant.args(pool))
		if got.IsError && isBudgetRefusal(got.Text) {
			t.Fatalf("%s spent the call budget, so the rest would read as measured: %s", variant.row, got.Text)
		}
		if run, ran := recorded.take(variant.verb); ran {
			measured.addPath(variant.verb, run.Checks)
		}
	}
}

// baseRow is the row a kind variant varies, or the row itself.
func baseRow(row string) string {
	base, _, _ := strings.Cut(row, "[")
	return base
}
