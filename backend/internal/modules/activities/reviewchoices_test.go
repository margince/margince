// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestReviewChoicesRefuseUnknownRepeatedAndMissingValues(t *testing.T) {
	questions := []ReviewQuestion{{Key: "reasons", Label: "Reasons", Type: "multiselect", Required: true, Options: []string{"Fit, scope", "Trust"}}}
	for _, selected := range [][]string{nil, {"Unknown"}, {"Trust", "Trust"}} {
		if err := validateReviewAnswers(questions, nil, map[string][]string{"reasons": selected}); err == nil {
			t.Errorf("accepted invalid selection %v", selected)
		}
	}
	if err := validateReviewAnswers(questions, nil, map[string][]string{"reasons": {"Fit, scope", "Trust"}}); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewAnswers(questions, map[string]string{"reasons": "Trust"}, nil); err == nil {
		t.Fatal("accepted scalar answer for multiple-choice question")
	}
	if err := validateReviewAnswers(questions, nil, map[string][]string{"invented": {"Trust"}}); err == nil {
		t.Fatal("accepted unknown question")
	}
}

func TestReviewQuestionsRefuseAmbiguousDefinitions(t *testing.T) {
	for _, questions := range [][]ReviewQuestion{
		nil,
		{{Key: "q", Label: "Q", Type: "other"}},
		{{Key: "q", Label: "Q", Type: "multiselect"}},
		{{Key: "q", Label: "Q", Type: "multiselect", Options: []string{"A", "A"}}},
		{{Key: "q", Label: "Q", Type: "text"}, {Key: "q", Label: "Other", Type: "text"}},
	} {
		if err := validateReviewQuestions(questions); err == nil {
			t.Errorf("accepted ambiguous definition %v", questions)
		}
	}
}
