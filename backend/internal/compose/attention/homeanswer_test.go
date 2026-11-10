// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"strings"
	"testing"
	"time"

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
