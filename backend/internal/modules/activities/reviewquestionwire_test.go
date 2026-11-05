// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"testing"
)

// A text question has no choices, so its wire form carries no `options` key.
// The contract declares an array, and an empty Go slice is written null.
func TestATextQuestionIsWrittenWithoutOptionsAndAChoiceQuestionKeepsThem(t *testing.T) {
	wire, err := json.Marshal(wireQuestions([]ReviewQuestion{
		{Key: "why", Label: "Why", Type: "text"},
		{Key: "reasons", Label: "Reasons", Type: "multiselect", Options: []string{"Fit", "Trust"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var questions []map[string]any
	if err := json.Unmarshal(wire, &questions); err != nil {
		t.Fatal(err)
	}
	if options, present := questions[0]["options"]; present {
		t.Errorf("the text question carries options %v, want the key left off", options)
	}
	if options, ok := questions[1]["options"].([]any); !ok || len(options) != 2 {
		t.Errorf("the choice question carries options %v, want its two choices", questions[1]["options"])
	}
}
