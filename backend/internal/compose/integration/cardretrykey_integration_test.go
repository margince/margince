// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A choice card's button asks its host to run change_tags, decide_approvals or
// resolve_duplicates with a retry key. That press is an ordinary tools/call.
// This test pins the registry's claim and replay for a card-reachable tool.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACardPressRepeatedUnderOneKeyCoinsOneTag(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	coin := func(key string) (json.RawMessage, error) {
		args := map[string]any{"action": "create", "name": "card-retry-word"}
		if key != "" {
			args["idempotency_key"] = key
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatalf("encoding the call: %v", err)
		}
		return registry.Invoke(ctx, "change_tags", encoded)
	}

	first, err := coin("press-1")
	if err != nil {
		t.Fatalf("the first press: %v", err)
	}
	retry, err := coin("press-1")
	if err != nil {
		t.Fatalf("the retry of that press: %v", err)
	}
	if string(retry) != string(first) {
		t.Fatalf("the retry answered\n%s\nwant the first press's answer\n%s", retry, first)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag WHERE name = $1`, "card-retry-word"); n != 1 {
		t.Fatalf("one press and its retry coined %d tags, want 1", n)
	}
	if _, err := coin(""); err == nil {
		t.Fatal("a keyless repeat was replayed as the first press; only a repeated key may be")
	}
}
