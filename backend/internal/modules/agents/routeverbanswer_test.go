// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"testing"
)

// A route's status may be a word, and the same write's cut answer carries the
// HTTP code. The schema has to admit both, or the answer loses its structured form.
func TestAWriteAnswerAdmitsARoutesOwnStatusAndTheCutOne(t *testing.T) {
	words := writeAnswerSchema(json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}}}`))
	for _, answer := range []string{`{"status":"open"}`, `{"status":200,"truncated":true}`} {
		if defect := ResultDefect(words, json.RawMessage(answer)); defect != "" {
			t.Errorf("%s against a route with a word status: %s", answer, defect)
		}
	}
	silent := writeAnswerSchema(json.RawMessage(`{"type":"object"}`))
	if defect := ResultDefect(silent, json.RawMessage(`{"status":204}`)); defect != "" {
		t.Errorf("a status-only answer: %s", defect)
	}
	if defect := ResultDefect(silent, json.RawMessage(`{"status":"open"}`)); defect == "" {
		t.Error("a route that declares no status answered a word, and the schema admitted it")
	}
}
