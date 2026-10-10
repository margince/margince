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

// toolRefused asks a tool the door must refuse or stage, and answers the
// refusal.
//
//craft:ignore naked-any a tool argument is whichever JSON value its schema declares
func (d *twoDoors) toolRefused(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	return d.mcp.CallRefused(t, name, args)
}

// effect is what a write left on one entity. It has one line per audit row,
// naming the action, the actor and the fields the row changed. It also holds
// the event types of its outbox rows. Two doors that run the same write leave
// the same lines.
type effect struct {
	Audit  []string
	Events []string
}

// auditKeysIgnored are the members every write moves, which say nothing about
// which write ran.
var auditKeysIgnored = map[string]bool{"id": true, "version": true, "created_at": true, "updated_at": true}

func changedKeys(before, after map[string]json.RawMessage) []string {
	var keys []string
	for key, value := range after {
		if auditKeysIgnored[key] {
			continue
		}
		if prior, held := before[key]; !held || string(prior) != string(value) {
			keys = append(keys, key)
		}
	}
	for key := range before {
		if _, held := after[key]; !held && !auditKeysIgnored[key] {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// auditImage decodes one audit image. A create has no before image and an
// erase no after image, so an empty one is the nil map.
func auditImage(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var image map[string]json.RawMessage
	if err := json.Unmarshal(raw, &image); err != nil {
		t.Fatalf("an audit image does not decode: %v\n%s", err, raw)
	}
	return image
}

func (d *twoDoors) effectOf(t *testing.T, entity string) effect {
	t.Helper()
	var out effect
	rows, err := d.e.Owner.Query(t.Context(),
		`SELECT action, actor_type, before, after FROM audit_log WHERE entity_id = $1 ORDER BY occurred_at, id`, entity)
	if err != nil {
		t.Fatalf("reading the audit rows of %s: %v", entity, err)
	}
	defer rows.Close()
	for rows.Next() {
		var action, actor string
		var before, after []byte
		if err := rows.Scan(&action, &actor, &before, &after); err != nil {
			t.Fatal(err)
		}
		prior, next := auditImage(t, before), auditImage(t, after)
		out.Audit = append(out.Audit, action+"/"+actor+"/"+strings.Join(changedKeys(prior, next), ","))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	events, err := d.e.Owner.Query(t.Context(),
		`SELECT coalesce(envelope->>'type', envelope->>'event_type', stream) FROM event_outbox
		 WHERE envelope::text LIKE '%' || $1 || '%' ORDER BY seq`, entity)
	if err != nil {
		t.Fatalf("reading the outbox rows of %s: %v", entity, err)
	}
	defer events.Close()
	for events.Next() {
		var kind string
		if err := events.Scan(&kind); err != nil {
			t.Fatal(err)
		}
		out.Events = append(out.Events, kind)
	}
	if err := events.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sameEffect fails when the two entities were not left alike, and says what
// each carries.
func (d *twoDoors) sameEffect(t *testing.T, what, viaREST, viaTool string) effect {
	t.Helper()
	rest, tool := d.effectOf(t, viaREST), d.effectOf(t, viaTool)
	if len(rest.Audit) == 0 {
		t.Fatalf("%s: the route left no audit row on %s", what, viaREST)
	}
	if !slices.Equal(rest.Audit, tool.Audit) || !slices.Equal(rest.Events, tool.Events) {
		t.Errorf("%s: the route left %+v, the tool left %+v", what, rest, tool)
	}
	return rest
}

// staged is one approval a door staged, with the target's id taken out of the
// text so two doors staging against two records read alike.
type staged struct {
	Kind, Status, TargetType, Summary, Change string
}

func (d *twoDoors) stagedOn(t *testing.T, target string) []staged {
	t.Helper()
	rows, err := d.e.Owner.Query(t.Context(),
		`SELECT kind, status, coalesce(target_entity_type, ''), coalesce(summary, ''), proposed_change::text
		 FROM approval WHERE target_entity_id = $1 ORDER BY created_at, id`, target)
	if err != nil {
		t.Fatalf("reading the approvals staged on %s: %v", target, err)
	}
	defer rows.Close()
	var out []staged
	for rows.Next() {
		var s staged
		if err := rows.Scan(&s.Kind, &s.Status, &s.TargetType, &s.Summary, &s.Change); err != nil {
			t.Fatal(err)
		}
		s.Summary = strings.ReplaceAll(s.Summary, target, "<target>")
		s.Change = strings.ReplaceAll(s.Change, target, "<target>")
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sameStagedKind fails unless each door staged one approval of the same kind
// against the same kind of record. The sentence a human reads and the change it
// replays are written by each door's own staging path. So they are left out of
// the comparison, and both must only be present.
func (d *twoDoors) sameStagedKind(t *testing.T, what, viaREST, viaTool string) staged {
	t.Helper()
	onREST, onTool := d.stagedOn(t, viaREST), d.stagedOn(t, viaTool)
	if len(onREST) != 1 || len(onTool) != 1 {
		t.Fatalf("%s: the route staged %d approvals, the tool %d, want one each", what, len(onREST), len(onTool))
	}
	rest, tool := onREST[0], onTool[0]
	if rest.Kind != tool.Kind || rest.Status != tool.Status || rest.TargetType != tool.TargetType {
		t.Errorf("%s: the route staged\n%+v\nthe tool staged\n%+v", what, rest, tool)
	}
	if rest.Summary == "" || tool.Summary == "" || rest.Change == "" || tool.Change == "" {
		t.Errorf("%s: a staged approval lacks its sentence or its change: route %+v, tool %+v", what, rest, tool)
	}
	return rest
}
