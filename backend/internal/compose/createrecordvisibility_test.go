// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

// An agent repeats what the create_record text says about who sees a new
// contact. The text is read from the served catalog, and the visibility from
// the constant the contacts store writes, so neither can change alone.
func TestCreateRecordStatesTheContactVisibility(t *testing.T) {
	// How the text words each visibility a typed contact create could set.
	contactVisibilityWording := map[string]string{
		"workspace": "A CONTACT created here is visible to everyone in the workspace",
		"owner":     "A CONTACT created here is visible to the human you are acting for and to nobody else",
	}
	description := servedToolDescription(t, "create_record")
	want, ok := contactVisibilityWording[contacts.TypedCreateVisibility]
	if !ok {
		t.Fatalf("no create_record wording for visibility %q: add one here and state it in the tool text",
			contacts.TypedCreateVisibility)
	}
	for visibility, wording := range contactVisibilityWording {
		if stated := strings.Contains(description, wording); stated != (wording == want) {
			t.Errorf("create_record states the %s wording = %v, but a typed contact create writes %q",
				visibility, stated, contacts.TypedCreateVisibility)
		}
	}
}

// servedToolDescription reads one tool's description from the published
// catalog, which TestPublishedMCPSurfaceMatchesWhatAClientIsServed holds to the
// served surface.
func servedToolDescription(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(mcpInfoDoc)
	if err != nil {
		t.Fatalf("reading the published catalog: %v", err)
	}
	var doc struct {
		Tools struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the published catalog: %v", err)
	}
	for _, tool := range doc.Tools.Tools {
		if tool.Name == name {
			return tool.Description
		}
	}
	t.Fatalf("the published catalog has no %s tool", name)
	return ""
}
