// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// TestTheChatEngineReadsActivityTextOnlyThroughSharedActivity holds the
// audience rule's one door. A second query of the activity table in the chat
// engine could post text the workspace may not read.
func TestTheChatEngineReadsActivityTextOnlyThroughSharedActivity(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(lit.Value)
				if err == nil && strings.Contains(text, "FROM activity a") {
					readers[file+":"+fn.Name.Name] = true
				}
				return true
			})
		}
	}
	if len(readers) != 1 || !readers["sync.go:sharedActivity"] {
		t.Fatalf("activity text is read by %v; only sync.go:sharedActivity may read it", readers)
	}
}

// TestEveryEventOfALinkedRecordIsTieredOrIgnored fails when the catalog gains
// an event on a linked record's kind that the engine neither tiers nor ignores.
func TestEveryEventOfALinkedRecordIsTieredOrIgnored(t *testing.T) {
	conditional := map[string]bool{"activity.captured": true, "activity.updated": true}
	for _, eventType := range events.Types() {
		entity, _, err := events.SplitType(eventType)
		if err != nil {
			t.Fatal(err)
		}
		if _, linked := chatRecordQueries[entity]; !linked || chatIgnoredEvents[eventType] || conditional[eventType] {
			continue
		}
		tier, _, posts := chatTier(events.Envelope{Type: eventType, Payload: json.RawMessage(`{}`)})
		if !posts || tier == "" {
			t.Errorf("%s belongs to a linked record but has no tier and is not ignored", eventType)
		}
	}
}

func TestAMilestoneLevelPostsOnlyMilestones(t *testing.T) {
	stage := events.Envelope{Type: "deal.stage_changed", Payload: json.RawMessage(`{"to_stage_name":"Negotiation"}`)}
	tier, detail, posts := chatTier(stage)
	if !posts || tier != chatlinks.TierMilestone || detail["to_stage_name"] != "Negotiation" {
		t.Fatalf("stage change = (%q, %v, %v), want a milestone naming the stage", tier, detail, posts)
	}
	edit := events.Envelope{Type: "deal.updated", Payload: json.RawMessage(`{"changed_fields":{"description":"x"}}`)}
	if tier, _, _ := chatTier(edit); tier != chatlinks.TierEverything {
		t.Fatalf("a description edit is tier %q, want everything", tier)
	}
	close := events.Envelope{Type: "deal.updated", Payload: json.RawMessage(`{"changed_fields":{"expected_close_date":"2026-12-01"}}`)}
	if tier, _, _ := chatTier(close); tier != chatlinks.TierImportant {
		t.Fatalf("a close date change is tier %q, want important", tier)
	}
	if _, _, posts := chatTier(events.Envelope{Type: "deal.created"}); posts {
		t.Fatal("deal.created posted, but no channel can exist before the deal")
	}
}
