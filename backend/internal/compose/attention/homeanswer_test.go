// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"strings"
	"testing"
	"time"
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
