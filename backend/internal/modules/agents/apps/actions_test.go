// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var frontendViews = filepath.Join("..", "..", "..", "..", "..", "frontend", "src", "mcp-apps")

func TestTheActionListsMatchTheFrontend(t *testing.T) {
	authored, err := os.ReadFile(filepath.Join(frontendViews, "actions.json"))
	if err != nil {
		t.Fatalf("reading the authored action lists: %v", err)
	}
	if string(authored) != string(actionsJSON) {
		t.Fatal("the embedded action lists differ from frontend/src/mcp-apps/actions.json. " +
			"Run `make -C backend mcp-apps-vocab` and commit the result")
	}
}

func TestEveryActionListNamesAViewThatExists(t *testing.T) {
	byView, err := viewActions()
	if err != nil {
		t.Fatalf("the action lists do not decode: %v", err)
	}
	for dir, tools := range byView {
		if _, err := os.Stat(filepath.Join(frontendViews, dir, "index.html")); err != nil {
			t.Errorf("actions.json lists %q, which is not a view directory", dir)
		}
		if len(tools) == 0 {
			t.Errorf("actions.json lists %q with no tool: omit a view that has no action", dir)
		}
	}
}

func TestACatalogViewReadsItsOwnActions(t *testing.T) {
	v := view{uri: "ui://margince/some-card.html"}
	if got := viewDir(v.uri); got != "some-card" {
		t.Fatalf("viewDir = %q, want some-card", got)
	}
	if len(v.actions()) != 0 {
		t.Fatalf("a view absent from actions.json has actions: %v", v.actions())
	}
}

func TestAdmitWaivesTheCallMethodOnlyForAViewWithActions(t *testing.T) {
	doc := cleanDocument + `<script>post({method:"tools/call"})</script>`
	if findings, _ := admit(doc, "Morning brief", false); !namesToken(findings, "tools/call") {
		t.Fatalf("a view without actions was admitted carrying the call method: %v", findings)
	}
	if findings, _ := admit(doc, "Morning brief", true); len(findings) != 0 {
		t.Fatalf("a view with actions was refused: %v", findings)
	}
	for _, token := range []string{"ui/tool", "Authorization"} {
		if findings, _ := admit(cleanDocument+token, "Morning brief", true); len(findings) == 0 {
			t.Errorf("a view with actions was admitted carrying %q", token)
		}
	}
}

func namesToken(findings []string, token string) bool {
	for _, f := range findings {
		if strings.EqualFold(f, token) {
			return true
		}
	}
	return false
}
