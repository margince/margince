// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/auth"
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
	if fields, named := kindFields[c.verb][c.kind]; named {
		args["fields"] = fields(pool)
	}
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

// kindFields are the fields a record tool takes on each further kind, as the
// fixture's fields are a company's. Without them the call on that kind stops
// at its arguments, and its checks are never asked.
//
//craft:ignore naked-any a field takes whichever JSON type its record's schema names
var kindFields = map[string]map[string]func(*recordPool) map[string]any{
	"create_record": {
		"contact": func(*recordPool) map[string]any { return map[string]any{"full_name": "Role Sweep", "source": "manual"} },
		"lead": func(*recordPool) map[string]any {
			return map[string]any{"full_name": "Role Sweep", "email": uniqueEmail(), "source": "manual"}
		},
		"project": func(p *recordPool) map[string]any {
			return map[string]any{"name": "Role Sweep", "company_id": p.take("company"), "source": "manual"}
		},
	},
	"update_record": {
		"contact": func(*recordPool) map[string]any { return map[string]any{"full_name": "Role Sweep Renamed"} },
		"lead":    func(*recordPool) map[string]any { return map[string]any{"full_name": "Role Sweep Renamed"} },
		"project": func(*recordPool) map[string]any { return map[string]any{"name": "Role Sweep Renamed"} },
		"deal":    func(*recordPool) map[string]any { return map[string]any{"name": "Role Sweep Renamed"} },
	},
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

// measureEveryKind calls each kind variant as the admin, and adds the checks
// of each that ran to its verb's paths. tools/list then offers a verb to a
// caller who holds every check of some path. A role refused on the call's own
// kind then calls each kind the admin ran. It is allowed the row where one
// runs, since the row is the tool and the listing offers a tool on any kind.
func measureEveryKind(t *testing.T, seats []roleSeat, recorded *toolCheckRecorder, measured toolChecksMeasure, variants []toolRoleCall, verdicts map[string]toolRoleVerdicts) {
	t.Helper()
	if len(variants) == 0 {
		t.Fatal("no call picks its record by a kind argument, so no path but the company's was measured")
	}
	admin := slices.IndexFunc(seats, func(seat roleSeat) bool { return seat.role.Key == "admin" })
	if admin < 0 {
		t.Fatal("no seeded role is the admin, so no kind's checks can be measured")
	}
	var adminRan []toolRoleCall
	callVariants(t, seats[admin], recorded, variants, func(variant toolRoleCall, got apptest.MCPResult, run auth.RecordedCall, ran bool) {
		if ran && !got.IsError {
			measured.addPath(variant.verb, run.Checks)
			adminRan = append(adminRan, variant)
		}
	})
	ranRows := make([][]string, len(seats))
	eachSeat(t, "kinds", seats, func(t *testing.T, i int, seat roleSeat) {
		refused := slices.DeleteFunc(slices.Clone(adminRan), func(variant toolRoleCall) bool {
			return !slices.Contains(verdicts[baseRow(variant.row)].Refused, seat.role.Key)
		})
		if len(refused) == 0 {
			return
		}
		callVariants(t, seat, recorded, refused, func(variant toolRoleCall, got apptest.MCPResult, _ auth.RecordedCall, _ bool) {
			if !got.IsError {
				ranRows[i] = append(ranRows[i], baseRow(variant.row))
			}
		})
	})
	for i, seat := range seats {
		for _, base := range ranRows[i] {
			row := verdicts[base]
			if slices.Contains(row.Refused, seat.role.Key) {
				row.Refused = slices.DeleteFunc(row.Refused, func(key string) bool { return key == seat.role.Key })
				row.Allowed = append(row.Allowed, seat.role.Key)
				verdicts[base] = row
			}
		}
	}
}

// callVariants calls each variant as a passport on the seat's role, on records
// seeded for it. It hands each answer, and what the gates noted, to answered.
func callVariants(t *testing.T, seat roleSeat, recorded *toolCheckRecorder, variants []toolRoleCall, answered func(toolRoleCall, apptest.MCPResult, auth.RecordedCall, bool)) {
	t.Helper()
	apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	clients := passportClients(t, seat.e, seat.role.Key+" kinds", 1+len(variants)/toolRoleCallsPerPassport)
	pool := seedToolRoleRecords(t, seat.e, variants, "")
	apptest.AssignRoleTo(t, seat.e, seat.email, seat.role.Key)
	defer apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	for n, variant := range variants {
		client := clients[n/toolRoleCallsPerPassport]
		pool.caller = client.Bearer()
		got, _ := callThroughApproval(t, seat.e, client, variant.verb, variant.args(pool))
		if got.IsError && isBudgetRefusal(got.Text) {
			t.Fatalf("%s as %s spent the call budget, so the rest would read as measured: %s", variant.row, seat.role.Key, got.Text)
		}
		run, ran := recorded.take(variant.verb, seat.human)
		answered(variant, got, run, ran)
	}
}

// baseRow is the row a kind variant varies, or the row itself.
func baseRow(row string) string {
	base, _, _ := strings.Cut(row, "[")
	return base
}
