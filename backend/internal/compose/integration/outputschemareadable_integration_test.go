// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/pkg/extension"
)

// Every result is checked against its tool's output schema. A schema the
// checker cannot read turns every answer of that tool into a logged defect.
// Every core tool the registry serves has one the checker reads; a unit's verb
// is refused at boot without one (buildExtensionTools).
func TestEveryServedOutputSchemaIsReadableByTheChecker(t *testing.T) {
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	checked := 0
	for _, spec := range specs {
		if len(spec.OutputSchema) == 0 {
			continue
		}
		checked++
		if err := agents.SchemaReadable(spec.OutputSchema); err != nil {
			t.Errorf("%s: %v", spec.Name, err)
		}
	}
	if checked == 0 {
		t.Fatal("no served tool declares an output schema, so nothing was read")
	}
}

// A unit verb whose output schema the checker cannot read is refused at boot,
// before RegisterExtensions applies anything, so a refused call leaves nothing.
func TestAUnitVerbWithAnUnreadableOutputSchemaIsRefusedAtBoot(t *testing.T) {
	verb := extension.Verb{
		Unit: "demo", Contract: "crm.yaml", OperationID: "listNotesOp", Route: "/ext/demo/list-notes",
		Method: http.MethodPost, Tool: "list_notes", Version: "1.0.0",
		Description: "A stand-in unit tool, described so the composition has something to serve.",
		Tier:        extension.TierAutoExecute, RequestedScope: extension.ScopeRead,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"note":{"type":["string","null"]}}}`),
	}
	if err := compose.RegisterExtensions(nil, []extension.Verb{verb}, nil); err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Fatalf("the unreadable output schema was admitted: %v", err)
	}
}
