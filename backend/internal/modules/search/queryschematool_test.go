// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"encoding/json"
	"testing"
)

// The tool door and the resource door answer the SAME BYTES.
//
// This is the claim VocabularyDocument's comment makes, and it is the whole
// reason a second door was safe to add: a hand-written vocabulary in the tool
// would be a maintained restatement of what the resolver derives, stale the
// first time a workspace declared a custom field. Two doors over one
// composition cost nothing; two compositions would cost correctness, and the
// difference is only visible if something checks.
func TestBothDoorsAnswerOneVocabulary(t *testing.T) {
	r := NewQuerySchemaResource(NewVocabularyResolver())
	ctx := context.Background()

	contents, err := r.ReadResource(ctx, QuerySchemaURI)
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	doc, err := r.VocabularyDocument(ctx)
	if err != nil {
		t.Fatalf("VocabularyDocument: %v", err)
	}
	if string(doc) != contents.Text {
		t.Errorf("the tool and the resource answer different documents, so a client "+
			"reading one learns a vocabulary the other does not admit.\ntool:     %s\nresource: %s",
			doc, contents.Text)
	}
	// And it is a document, not an empty answer dressed as one: a caller
	// reading `{}` would conclude this workspace admits nothing.
	var shape struct {
		Version string          `json:"version"`
		Targets json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(doc, &shape); err != nil {
		t.Fatalf("the vocabulary is not a JSON document: %v", err)
	}
	if shape.Version == "" {
		t.Error("the vocabulary carries no version, which is the value a plan's own version must match")
	}
	if len(shape.Targets) == 0 {
		t.Error("the vocabulary names no targets member, so a caller cannot tell an empty " +
			"estate from a document that was never composed")
	}
}
