// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Only the question Home opens on is cached, under one key for its spellings.
func TestOnlyHomesOwnQuestionIsKeptAndItsSpellingsShareOneKey(t *testing.T) {
	now := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC)
	svc := &Service{now: func() time.Time { return now }}
	home := worklistQuestion{view: "today", scope: "mine", filter: "all", limit: 100, omitRelationships: true}

	key, ok, err := svc.homeKey(context.Background(), home)
	if err != nil || !ok {
		t.Fatalf("Home's own question: kept %v (err %v), want it kept", ok, err)
	}
	if !strings.Contains(key, "day=2026-11-02") || !strings.Contains(key, "zone=UTC") {
		t.Errorf("key %q does not name the installation's zone and day", key)
	}
	implicit := home
	implicit.scope, implicit.filter = "", ""
	if other, _, _ := svc.homeKey(context.Background(), implicit); other != key {
		t.Errorf("the default spelling keys %q, Home's explicit one %q", other, key)
	}

	// A kept day is served for hours, so only the key keeps it off the next day.
	now = now.Add(time.Hour)
	if next, _, _ := svc.homeKey(context.Background(), home); next == key {
		t.Errorf("past midnight Home still asks under %q, so yesterday's kept day would be served", key)
	}

	for name, q := range map[string]worklistQuestion{
		"another view":       {view: "decisions", scope: "mine", filter: "all", omitRelationships: true},
		"another scope":      {view: "today", scope: "team", filter: "all", omitRelationships: true},
		"another filter":     {view: "today", scope: "mine", filter: "tasks", omitRelationships: true},
		"with relationships": {view: "today", scope: "mine", filter: "all"},
	} {
		if _, kept, err := svc.homeKey(context.Background(), q); err != nil || kept {
			t.Errorf("%s: kept %v (err %v), want it read fresh", name, kept, err)
		}
	}
}

func TestHomePresentationKeysKeepEachReadersLanguage(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	svc := &Service{now: func() time.Time { return now }}
	question := worklistQuestion{view: "today", scope: "mine", filter: "all", limit: 100, omitRelationships: true}
	keys := map[string]bool{}
	for _, language := range textlang.Shipped {
		ctx := systemtext.WithLocale(context.Background(), language)
		key, kept, err := svc.homeKey(ctx, question)
		if err != nil || !kept || keys[key] {
			t.Fatalf("language %s key %q: kept=%v, err=%v", language, key, kept, err)
		}
		keys[key] = true
		detached := context.WithoutCancel(ctx)
		refreshed, _, err := svc.homeKey(detached, question)
		if err != nil || refreshed != key {
			t.Fatalf("background refresh lost language %s: %q, %v", language, refreshed, err)
		}
	}
}

// A walk counts a group by its members and an offset counts it once. A kept
// page whose walk ended pages on from the ranked count, never the walk's.
func TestAKeptPageWhoseWalkEndedPagesOnFromItsRankedOffset(t *testing.T) {
	walk := walkState{id: ids.NewV7(), carried: true, offset: 2}
	token := encodeCursor(5, scopeMine, "", ids.UUID{}, walk, nil)
	page := crmcontracts.Worklist{NextCursor: &token, Walk: walk.state()}

	got, err := pagedWithoutWalk(page)
	if err != nil {
		t.Fatal(err)
	}
	if got.Walk != nil || got.NextCursor == nil {
		t.Fatalf("the page kept walk %v and cursor %v, want no walk and a cursor", got.Walk, got.NextCursor)
	}
	cursor, err := decodeCursor(*got.NextCursor, scopeMine, "", ids.UUID{})
	if err != nil {
		t.Fatalf("the cursor without the walk is refused: %v", err)
	}
	if cursor.At != 2 || !cursor.Snapshot.IsZero() || cursor.Ranked != 0 {
		t.Errorf("the cursor resumes at %d with walk %s and ranked %d, want 2, none and 0", cursor.At, cursor.Snapshot, cursor.Ranked)
	}

	unranked := walkState{id: walk.id, carried: true}
	for name, rest := range map[string]*backlogCursor{
		"without a backlog position": nil,
		"with a backlog position":    {At: map[string]string{"tasks": ""}},
	} {
		older := encodeCursor(5, scopeMine, "", ids.UUID{}, unranked, rest)
		if _, err := pagedWithoutWalk(crmcontracts.Worklist{NextCursor: &older}); !errors.Is(err, homecache.ErrReadFresh) {
			t.Errorf("an older cursor %s answered %v, want homecache.ErrReadFresh", name, err)
		}
	}
}
