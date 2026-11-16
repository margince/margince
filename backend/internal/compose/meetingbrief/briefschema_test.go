// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The brief request sends the shape ParseBriefSections reads. A constrained
// decoder cannot write a reply the parser refuses whole, and the schema admits
// every reply the parser keeps.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/schema"
)

func briefReply(kind, nature, entityType string) string {
	return `{"sections":[{"kind":"` + kind + `","sentences":[{"text":"They asked for a date.","nature":"` + nature +
		`","evidence":[{"entity_type":"` + entityType + `","entity_id":"` + meetingID + `"}]}]}]}`
}

func TestTheBriefRequestSendsTheShapeItsParserReads(t *testing.T) {
	in := fullInput()
	req := BriefRequest(in, string(textlang.English))
	if len(req.ResponseSchema) == 0 {
		t.Fatal("the brief request carries no response schema, so a decoder may write any shape")
	}
	good := briefReply("goal", natureFact, citeActivity)
	if kept, err := parseCitedSections(good, in); err != nil || len(kept) != 1 {
		t.Fatalf("the parser kept %d sections of the well-formed reply (%v), want 1", len(kept), err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, good); err != nil {
		t.Errorf("the schema refuses a reply the parser keeps: %v", err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, briefReply("goal", natureFact, "invoice")); err == nil {
		t.Error("the schema admits a citation of a record type no brief can ground")
	}
	if err := schema.ValidateJSON(req.ResponseSchema, briefReply("goal", "opinion", citeActivity)); err == nil {
		t.Error("the schema admits a nature no section keeps")
	}
	noEvidence := `{"sections":[{"kind":"goal","sentences":[{"text":"t","nature":"fact"}]}]}`
	if err := schema.ValidateJSON(req.ResponseSchema, noEvidence); err == nil {
		t.Error("the schema admits a sentence with no evidence key, so its keys are not required")
	}
}

// Every kind the schema lets the model write is one the parser keeps a
// sentence under, and a kind outside the list fails both.
func TestTheBriefSchemaAndParserAgreeOnSectionKinds(t *testing.T) {
	in := fullInput()
	req := BriefRequest(in, string(textlang.English))
	kinds := []string{"gossip"}
	for kind := range natureAllowed {
		kinds = append(kinds, string(kind))
	}
	for _, kind := range kinds {
		reply := briefReply(kind, natureFact, citeActivity)
		kept, err := parseCitedSections(reply, in)
		if err != nil {
			t.Fatalf("kind %q: the parser refused the reply whole: %v", kind, err)
		}
		admitted := schema.ValidateJSON(req.ResponseSchema, reply) == nil
		if admitted != (len(kept) == 1) {
			t.Errorf("kind %q: the schema admits it = %v, the parser keeps %d sections", kind, admitted, len(kept))
		}
	}
}
