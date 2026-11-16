// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package modelcontract defines the published model prompt document.
package modelcontract

import "encoding/json"

// Document is the published JSON.
type Document struct {
	// Sites lists the registered sites this build sends a prompt from.
	Sites []Entry `json:"sites"`
}

// Entry is one site, structured.
type Entry struct {
	Task      string `json:"task"`
	Site      string `json:"site"`
	Isolation string `json:"isolation"`
	// Systems holds the distinct instructions the case issued, boundary marker
	// canonicalised.
	Systems []string `json:"systems"`
	// AnswerSchema is the shape the completion must conform to, where the site
	// constrains it. Per-call record ids are replaced by a placeholder.
	AnswerSchema    json.RawMessage `json:"answer_schema,omitempty"`
	SpansInScenario int             `json:"spans_in_scenario"`
	// Calls is how many requests the case issued for one scenario.
	Calls int `json:"calls"`
	// A provider cannot reuse bytes after the boundary sentence, even when identical.
	SystemBytes        int `json:"system_bytes"`
	SystemTokens       int `json:"system_tokens_estimate"`
	RulesBytes         int `json:"rules_bytes"`
	BoundaryBytes      int `json:"boundary_bytes"`
	AfterBoundaryBytes int `json:"after_boundary_bytes"`
	CacheablePercent   int `json:"cacheable_percent"`
	// DecisionQuestions is the site's decision form, for a site whose task
	// declares one: the questions a decision model is asked before the ladder.
	// The state each one reads is per scenario and is not published.
	DecisionQuestions []DecisionQuestion `json:"decision_questions,omitempty"`
}

// DecisionQuestion is one question of a site's decision form, as its adapter
// builds it from the committed fixture.
type DecisionQuestion struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}
