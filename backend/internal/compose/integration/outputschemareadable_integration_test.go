// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
)

// Every result is checked against its tool's output schema. A schema the
// checker cannot read turns every answer of that tool into a logged defect.
// Every tool the composed registry serves has one the checker reads.
func TestEveryServedOutputSchemaIsReadableByTheChecker(t *testing.T) {
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	checked := 0
	for _, spec := range specs {
		if len(spec.OutputSchema) == 0 {
			continue
		}
		checked++
		// A null against a readable schema is a type defect, never an unreadable one.
		if defect := agents.ResultDefect(spec.OutputSchema, json.RawMessage(`null`)); strings.Contains(defect, "not readable") {
			t.Errorf("%s: %s", spec.Name, defect)
		}
	}
	if checked == 0 {
		t.Fatal("no served tool declares an output schema, so nothing was read")
	}
}
