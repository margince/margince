// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company-read answer's closed sets are one list each, read by the schema,
// the validator and the prompt alike.

import (
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// The prompt names the fields a change may touch in prose, and the schema
// closes the same set for the decoder. A field in one and not the other is
// either a field the model is told to use and cannot write, or one it may
// write and was never told about.
func TestTheCompanyReadPromptAndSchemaNameTheSameFields(t *testing.T) {
	const lead = "Use only these fields: "
	start := strings.Index(companyReadMessageSystem, lead)
	if start < 0 {
		t.Fatalf("the company-read prompt no longer lists its fields after %q", lead)
	}
	listed := companyReadMessageSystem[start+len(lead):]
	listed = listed[:strings.Index(listed, ".")]
	prompt := strings.Split(listed, ", ")
	slices.Sort(prompt)

	fields := companyReadChangeFields()
	for _, field := range fields {
		if !crmcontracts.CompanySiteReadSuggestedChangeField(field).Valid() {
			t.Errorf("the schema offers %q, which the contract's change enum refuses", field)
		}
	}
	slices.Sort(fields)
	if !slices.Equal(prompt, fields) {
		t.Errorf("the prompt lists %v and the schema closes the field to %v", prompt, fields)
	}
}

// Every kind the schema lets the model write is one the validator keeps, and a
// kind outside the list fails both.
func TestTheCompanyReadSchemaAndValidatorAgreeOnKinds(t *testing.T) {
	reply := func(kind string) string {
		return `{"kind":"` + kind + `","message":"m","proposed_changes":[],"source_ids":[]}`
	}
	for _, kind := range append(slices.Clone(companyConversationKinds), "chitchat") {
		schemaErr := schema.ValidateJSON(companyReadMessageSchema, reply(kind))
		if (schemaErr == nil) != companyConversationKindValid(kind) {
			t.Errorf("kind %q: the schema admits it = %v, the validator keeps it = %v",
				kind, schemaErr == nil, companyConversationKindValid(kind))
		}
	}
}
