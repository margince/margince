// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

// A surface's reason kinds are declared once, and both the response schema a
// model decodes under and the parse that keeps its reasons read that one list.

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

func narrowSurface() Surface {
	return Surface{
		Name:  "test draft",
		Kinds: []crmcontracts.AccountDraftReasonKind{crmcontracts.AccountDraftReasonKindIntent, crmcontracts.AccountDraftReasonKindDeal},
		KeepUncited: func(kind crmcontracts.AccountDraftReasonKind, _ string) bool {
			return kind == crmcontracts.AccountDraftReasonKindIntent
		},
		Cites: func(entityID string) string {
			if entityID == "deal-1" {
				return "deal"
			}
			return ""
		},
	}
}

func TestTheSchemaAdmitsExactlyTheKindsTheParseKeeps(t *testing.T) {
	surface := narrowSurface()
	shape := surface.schema()
	answer := func(kind string) string {
		return `{"subject":"s","body":"b","reasoning":[{"kind":"` + kind +
			`","label":"l","entity_type":null,"entity_id":null}]}`
	}
	for _, kind := range []string{"intent", "deal", "dossier", "invented"} {
		_, parsed := surface.kind(kind)
		schemaErr := schema.ValidateJSON(shape, answer(kind))
		if parsed != (schemaErr == nil) {
			t.Errorf("kind %q: the parse keeps it = %v, the schema admits it = %v (%v) — two answers to one list",
				kind, parsed, schemaErr == nil, schemaErr)
		}
	}
}

// The schema is closed at every level, and requires the citation pair to be
// present even when it is null — the only optional the strict profile takes.
func TestTheSchemaIsClosedAndSpellsANoCitationReasonAsNull(t *testing.T) {
	shape := narrowSurface().schema()
	for name, answer := range map[string]string{
		"an extra top-level key":    `{"subject":"s","body":"b","reasoning":[],"tone":"warm"}`,
		"an extra reason key":       `{"subject":"s","body":"b","reasoning":[{"kind":"intent","label":"l","entity_type":null,"entity_id":null,"score":1}]}`,
		"an omitted citation":       `{"subject":"s","body":"b","reasoning":[{"kind":"intent","label":"l"}]}`,
		"omitted reasoning":         `{"subject":"s","body":"b"}`,
		"a citation that is a list": `{"subject":"s","body":"b","reasoning":[{"kind":"deal","label":"l","entity_type":["deal"],"entity_id":"deal-1"}]}`,
	} {
		if err := schema.ValidateJSON(shape, answer); err == nil {
			t.Errorf("the schema admitted %s", name)
		}
	}
}

// A null citation decodes to "no citation", so an uncited intent survives and
// a cited deal keeps its pair.
func TestANullCitationReadsAsNoCitation(t *testing.T) {
	var out modelDraft
	raw := `{"subject":"s","body":"b","reasoning":[` +
		`{"kind":"intent","label":"asked for a call","entity_type":null,"entity_id":null},` +
		`{"kind":"deal","label":"renewal","entity_type":"deal","entity_id":"deal-1"}]}`
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	kept := keepGroundedReasons(narrowSurface(), out.Reasoning)
	if len(kept) != 2 || kept[0].EntityID != "" || kept[1].EntityID != "deal-1" {
		t.Fatalf("kept = %+v, want the uncited intent and the cited deal", kept)
	}
}

// A null type beside an id is not "no citation": it is a pair naming a record
// by id alone, and an id this draft never read resolves to the same empty
// string a null type decodes to. Either way the reason is dropped, never kept
// as a chip pointing at a record nobody checked.
func TestACitationWithANullTypeIsDropped(t *testing.T) {
	for name, entityID := range map[string]string{
		"an id this draft never read": "01a07000-0000-7000-8000-00000000dead",
		"an id it did read":           "deal-1",
	} {
		var out modelDraft
		raw := `{"subject":"s","body":"b","reasoning":[{"kind":"deal","label":"signed the renewal",` +
			`"entity_type":null,"entity_id":"` + entityID + `"}]}`
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if kept := keepGroundedReasons(narrowSurface(), out.Reasoning); len(kept) != 0 {
			t.Errorf("%s with a null type was kept as %+v", name, kept)
		}
	}
}
