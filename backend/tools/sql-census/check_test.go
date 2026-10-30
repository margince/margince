// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func rep(scope principal.RowScope) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		TeamIDs: []ids.UUID{ids.NewV7()},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: scope,
		},
	})
}

func renderGate(t *testing.T, ctx context.Context, gate func(context.Context, string, func(any) int) (string, error)) string {
	t.Helper()
	var args []any
	sql, err := gate(ctx, "a", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		t.Fatalf("rendering the gate: %v", err)
	}
	return sql
}

// The tracer drops statements under CensusMinBytes. If one gate fitted under
// that, a statement holding two could too, and the check would pass a census
// that never saw it.
func TestTheCensusRecordsEveryStatementThatHoldsAGate(t *testing.T) {
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll} {
		for name, gate := range map[string]func(context.Context, string, func(any) int) (string, error){
			"content": auth.ActivityContentClause, "audience": auth.ActivityAudienceArm,
		} {
			if got := len(renderGate(t, rep(scope), gate)); got < database.CensusMinBytes {
				t.Errorf("the %s gate under row scope %s renders %d bytes, under the census's %d", name, scope, got, database.CensusMinBytes)
			}
		}
	}
}

func twoGateStatement(t *testing.T, sender string) *statement {
	t.Helper()
	gate := renderGate(t, rep(principal.RowScopeTeam), auth.ActivityContentClause)
	return &statement{
		sql:    "SELECT a.id FROM activity a WHERE " + gate + " OR EXISTS (SELECT 1 FROM activity a WHERE " + gate + ")",
		chains: [][]string{{"backend/internal/thing_test.go:9 thing.TestRead", sender}},
	}
}

func TestACheckRefusesARepeatedGateFromASenderNotAllowed(t *testing.T) {
	repeats := repeatedGates([]*statement{twoGateStatement(t, "backend/internal/modules/activities/waiting.go:40 activities.readWaiting")})
	if len(repeats) != 1 || repeats[0].copies != 2 {
		t.Fatalf("repeats = %+v, want one sender with 2 copies", repeats)
	}
	if repeats[0].sender != "backend/internal/modules/activities/waiting.go activities.readWaiting" {
		t.Fatalf("sender = %q: the test frame was not skipped, or the line was kept", repeats[0].sender)
	}
	if err := judge(io.Discard, repeats, map[string]string{}); err == nil {
		t.Fatal("a repeated gate from an unlisted sender passed")
	}
	allow := map[string]string{repeats[0].sender: "read by the reminders lane"}
	if err := judge(io.Discard, repeats, allow); err != nil {
		t.Fatalf("an allowlisted sender was refused: %v", err)
	}
}

func TestOneGateIsNotARepeat(t *testing.T) {
	gate := renderGate(t, rep(principal.RowScopeOwn), auth.ActivityContentClause)
	stmt := &statement{sql: "SELECT 1 FROM activity a WHERE " + gate, chains: [][]string{{"backend/internal/x.go:1 x.Read"}}}
	if got := repeatedGates([]*statement{stmt}); len(got) != 0 {
		t.Fatalf("one gate counted as a repeat: %+v", got)
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnAllowlistEntryNeedsAReason(t *testing.T) {
	path := writeFile(t, t.TempDir(), "allow.txt", "backend/internal/x.go x.Read\n")
	if _, err := readAllow(path); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("an entry with no reason was read: %v", err)
	}
}

// One shard sees a slice of the lane, so an entry is stale only when no
// shard's seen file names it.
func TestStaleFailsOnAnEntryNoShardSawRepeat(t *testing.T) {
	dir := t.TempDir()
	allow := writeFile(t, dir, "allow.txt", "a.go a.Read still repeats\nb.go b.Read fixed since\n")
	first := writeFile(t, dir, "seen-1.txt", "a.go a.Read\n")
	second := writeFile(t, dir, "seen-2.txt", "")
	if err := runStale([]string{"-allow", allow, first, second}); err == nil {
		t.Fatal("an entry no shard saw repeat was not reported")
	}
	both := writeFile(t, dir, "seen-3.txt", "b.go b.Read\n")
	if err := runStale([]string{"-allow", allow, first, both}); err != nil {
		t.Fatalf("every entry was seen across the shards, yet: %v", err)
	}
}
