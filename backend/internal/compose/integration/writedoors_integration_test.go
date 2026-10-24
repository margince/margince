// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Helpers for the write tools that sit on a route of their own shape. A write
// has two answers to compare: what each door returns, and what each leaves
// behind. The second is read from the audit and outbox rows, which every
// write commits in the one transaction as the domain row.

import (
	"encoding/json"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// restStatus sends a route as the passport and fails unless the answer is the
// status the scenario names. It answers the body.
//
//craft:ignore naked-any the request body is whichever JSON shape the route under test takes
func (d *twoDoors) restStatus(t *testing.T, method, path string, body any, want int) json.RawMessage {
	t.Helper()
	var out json.RawMessage
	if status := d.e.Call(t, method, path, body, d.bearer, &out); status != want {
		t.Fatalf("%s %s as a passport → %d, want %d: %s", method, path, status, want, out)
	}
	return out
}

// restStatusWith is restStatus under headers the scenario chose.
//
//craft:ignore naked-any the request body is whichever JSON shape the route under test takes
func (d *twoDoors) restStatusWith(t *testing.T, method, path string, body any, headers map[string]string, want int) json.RawMessage {
	t.Helper()
	var out json.RawMessage
	if status := d.e.Call(t, method, path, body, headers, &out); status != want {
		t.Fatalf("%s %s → %d, want %d: %s", method, path, status, want, out)
	}
	return out
}

// restPinned is restStatus for a route that wants the version the caller read
// in If-Match.
//
//craft:ignore naked-any the request body is whichever JSON shape the route under test takes
func (d *twoDoors) restPinned(t *testing.T, method, path string, body any, version int) json.RawMessage {
	t.Helper()
	headers := map[string]string{"If-Match": strconv.Itoa(version)}
	maps.Copy(headers, d.bearer)
	var out json.RawMessage
	if status := d.e.Call(t, method, path, body, headers, &out); status != http.StatusOK {
		t.Fatalf("%s %s as a passport → %d, want 200: %s", method, path, status, out)
	}
	return out
}

// toolSeed runs a tool to set a scenario up. The both-doors census does not
// count it, because the scenario is not asking whether the doors agree on it.
//
//craft:ignore naked-any a tool argument is whichever JSON value its schema declares
func (d *twoDoors) toolSeed(t *testing.T, name string, args map[string]any) json.RawMessage {
	t.Helper()
	return d.tool(t, name, args)
}

// toolRefused asks a tool the door must refuse or stage, and answers the
// refusal.
//
//craft:ignore naked-any a tool argument is whichever JSON value its schema declares
func (d *twoDoors) toolRefused(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	return d.mcp.CallRefused(t, name, args)
}

// effect is what the passport's writes left on one entity. It has one line per
// audit row, naming the action, the actor, the passport, the authority and the
// values the row recorded. It also holds each outbox event the passport raised
// on the entity. Seeding is done by a human, so it stays out. Two doors that run
// the same write leave the same lines.
type effect struct {
	Audit  []string
	Events []string
}

var uuidText = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// volatileKey names the members two writes of the same kind never share: the
// ids and the moments they ran at.
func volatileKey(key string) bool {
	return key == "id" || key == "seq" || key == "version" || key == "correlation_id" || key == "trace_id" ||
		strings.HasSuffix(key, "_at")
}

// scrub rewrites a decoded JSON value so two entities compare alike. The
// entity's own id reads `<self>` and any other id `<id>`. A member in `differ`
// or a volatile one is dropped, and the rest keeps its value.
//
//craft:ignore naked-any a decoded JSON value has no narrower type
func scrub(value any, self string, differ map[string]bool) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, member := range v {
			if !differ[key] && !volatileKey(key) {
				out[key] = scrub(member, self, differ)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, member := range v {
			out[i] = scrub(member, self, differ)
		}
		return out
	case string:
		return uuidText.ReplaceAllString(strings.ReplaceAll(v, self, "<self>"), "<id>")
	default:
		return value
	}
}

// canonical decodes raw JSON, scrubs it and writes it back with sorted members.
func canonical(t *testing.T, raw []byte, self string, differ map[string]bool) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("a recorded image does not decode: %v\n%s", err, raw)
	}
	out, err := json.Marshal(scrub(value, self, differ))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func (d *twoDoors) effectOf(t *testing.T, entity string, differ map[string]bool) effect {
	t.Helper()
	var out effect
	rows, err := d.e.Owner.Query(t.Context(),
		`SELECT action, actor_type, actor_id, coalesce(passport_id::text, ''), coalesce(on_behalf_of::text, ''),
		        coalesce(authorization_rule, ''), coalesce(evidence::text, ''), coalesce(before::text, ''), coalesce(after::text, '')
		 FROM audit_log WHERE entity_id = $1 AND actor_type = 'agent' ORDER BY occurred_at, id`, entity)
	if err != nil {
		t.Fatalf("reading the audit rows of %s: %v", entity, err)
	}
	defer rows.Close()
	for rows.Next() {
		var action, actorType, actorID, passport, onBehalf, rule, evidence, before, after string
		if err := rows.Scan(&action, &actorType, &actorID, &passport, &onBehalf, &rule, &evidence, &before, &after); err != nil {
			t.Fatal(err)
		}
		out.Audit = append(out.Audit, strings.Join([]string{
			action, actorType, actorID, passport, onBehalf, rule,
			canonical(t, []byte(evidence), entity, differ),
			canonical(t, []byte(before), entity, differ),
			canonical(t, []byte(after), entity, differ),
		}, " | "))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	events, err := d.e.Owner.Query(t.Context(),
		`SELECT stream, envelope::text FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text AND envelope->'actor'->>'type' = 'agent' ORDER BY seq`, entity)
	if err != nil {
		t.Fatalf("reading the outbox rows of %s: %v", entity, err)
	}
	defer events.Close()
	for events.Next() {
		var stream, envelope string
		if err := events.Scan(&stream, &envelope); err != nil {
			t.Fatal(err)
		}
		out.Events = append(out.Events, stream+" | "+canonical(t, []byte(envelope), entity, differ))
	}
	if err := events.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sameEffect fails when the two entities were not left alike, and says what
// each carries. `differ` names the members the two records differ in by how the
// scenario seeded them, such as a name.
func (d *twoDoors) sameEffect(t *testing.T, what, viaREST, viaTool string, differ ...string) effect {
	t.Helper()
	skip := map[string]bool{}
	for _, key := range differ {
		skip[key] = true
	}
	rest, tool := d.effectOf(t, viaREST, skip), d.effectOf(t, viaTool, skip)
	if len(rest.Audit) == 0 {
		t.Fatalf("%s: the route left no audit row on %s", what, viaREST)
	}
	if !slices.Equal(rest.Audit, tool.Audit) {
		t.Errorf("%s: the route left audit rows\n%s\nthe tool left\n%s", what, strings.Join(rest.Audit, "\n"), strings.Join(tool.Audit, "\n"))
	}
	if !slices.Equal(rest.Events, tool.Events) {
		t.Errorf("%s: the route left events\n%s\nthe tool left\n%s", what, strings.Join(rest.Events, "\n"), strings.Join(tool.Events, "\n"))
	}
	return rest
}

// sameIDs fails when the route and the tool name different records. Order is not
// compared, because the two doors page their lists their own way.
func sameIDs(t *testing.T, what string, rest, tool []string) {
	t.Helper()
	if got, want := slices.Sorted(slices.Values(rest)), slices.Sorted(slices.Values(tool)); !slices.Equal(got, want) {
		t.Errorf("%s: the route names %v and the tool %v", what, got, want)
	}
}

// sameStored fails when two stored records are not alike in every member but
// the ones the scenario seeded differently (`differ`), the ids, and the moments.
// It reads the whole record, so a value one door wrote wrongly cannot hide in a
// member a thin struct left out.
func sameStored(t *testing.T, what string, rest, tool json.RawMessage, viaREST, viaTool string, differ ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, key := range differ {
		skip[key] = true
	}
	if got, want := canonical(t, rest, viaREST, skip), canonical(t, tool, viaTool, skip); got != want {
		t.Errorf("%s: the route stored\n%s\nthe tool stored\n%s", what, got, want)
	}
}

// staged is one approval a door staged: what a human acts on and what the
// system pins the decision to. The sentence a human reads and the change it
// replays are held apart, because each door's staging path writes its own.
type staged struct {
	Kind, Status, TargetType, TargetVersion, CoTargetType, CoTargetID, TargetLabel string
	Summary, Change                                                                string
}

func (d *twoDoors) stagedOn(t *testing.T, target string) []staged {
	t.Helper()
	rows, err := d.e.Owner.Query(t.Context(),
		`SELECT kind, status, coalesce(target_entity_type, ''), coalesce(target_version::text, ''),
		        coalesce(co_target_entity_type, ''), coalesce(co_target_entity_id::text, ''),
		        coalesce(target_label, ''), coalesce(summary, ''), proposed_change::text
		 FROM approval WHERE target_entity_id = $1 ORDER BY created_at, id`, target)
	if err != nil {
		t.Fatalf("reading the approvals staged on %s: %v", target, err)
	}
	defer rows.Close()
	var out []staged
	for rows.Next() {
		var s staged
		if err := rows.Scan(&s.Kind, &s.Status, &s.TargetType, &s.TargetVersion, &s.CoTargetType, &s.CoTargetID, &s.TargetLabel, &s.Summary, &s.Change); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sameStaged fails unless each door staged one approval alike: kind, status,
// the pin, the merge's second record, and the label and sentence a human reads.
// The last two carry the target's name, which `names` gives per door. The route
// may follow the tool's sentence with body fields the command erased. The
// replayed change is each door's own, so it need only be present.
func (d *twoDoors) sameStaged(t *testing.T, what, viaREST, viaTool string, names ...string) staged {
	t.Helper()
	rest, tool := d.stagedRows(t, what, viaREST, viaTool, names)
	if rest.Summary != tool.Summary && !strings.HasPrefix(rest.Summary, tool.Summary+": ") {
		t.Errorf("%s: the route staged the sentence %q and the tool %q", what, rest.Summary, tool.Summary)
	}
	return differInNothingBut(t, what, rest, tool)
}

// stagedRows reads the one approval each door staged, with the target's name
// masked, and fails when a sentence or a change is missing.
func (d *twoDoors) stagedRows(t *testing.T, what, viaREST, viaTool string, names []string) (rest, tool staged) {
	t.Helper()
	onREST, onTool := d.stagedOn(t, viaREST), d.stagedOn(t, viaTool)
	if len(onREST) != 1 || len(onTool) != 1 {
		t.Fatalf("%s: the route staged %d approvals, the tool %d, want one each", what, len(onREST), len(onTool))
	}
	rest, tool = onREST[0], onTool[0]
	if len(names) == 2 {
		rest.TargetLabel = strings.ReplaceAll(rest.TargetLabel, names[0], "<name>")
		tool.TargetLabel = strings.ReplaceAll(tool.TargetLabel, names[1], "<name>")
		rest.Summary = strings.ReplaceAll(rest.Summary, names[0], "<name>")
		tool.Summary = strings.ReplaceAll(tool.Summary, names[1], "<name>")
	}
	if rest.Summary == "" || tool.Summary == "" || rest.Change == "" || tool.Change == "" {
		t.Errorf("%s: a staged approval lacks its sentence or its change: route %+v, tool %+v", what, rest, tool)
	}
	return rest, tool
}

// differInNothingBut fails when two rows differ in a member other than the
// sentence and the replayed change, which each door writes its own way.
func differInNothingBut(t *testing.T, what string, rest, tool staged) staged {
	t.Helper()
	rest.Summary, rest.Change, tool.Summary, tool.Change = "", "", "", ""
	if rest != tool {
		t.Errorf("%s: the route staged\n%+v\nthe tool staged\n%+v", what, rest, tool)
	}
	return rest
}
