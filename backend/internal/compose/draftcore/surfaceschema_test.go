// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

// A surface's reason kinds are declared once, and both the response schema a
// model decodes under and the parse that keeps its reasons read that one list.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// deal1 is the one deal narrowSurface knows; its payload shows it as d1.
const deal1 = "01a07000-0000-7000-8000-000000000001"

func narrowSurface() Surface {
	return Surface{
		Name:  "test draft",
		Kinds: []crmcontracts.AccountDraftReasonKind{crmcontracts.AccountDraftReasonKindIntent, crmcontracts.AccountDraftReasonKindDeal},
		KeepUncited: func(kind crmcontracts.AccountDraftReasonKind, _ string) bool {
			return kind == crmcontracts.AccountDraftReasonKindIntent
		},
		Known: map[string]string{deal1: "deal"},
	}
}

// keepNarrow keeps reasons as narrowSurface's parse does, over a payload that
// shows its deal.
func keepNarrow(reasons []modelReason) []Reason {
	surface := narrowSurface()
	return keepGroundedReasons(surface, claims.MintKinds(`{"deal":"`+deal1+`"}`, surface.Known), reasons)
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
		`{"kind":"deal","label":"renewal","entity_type":"deal","entity_id":"d1"}]}`
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	kept := keepNarrow(out.Reasoning)
	if len(kept) != 2 || kept[0].EntityID != "" || kept[1].EntityID != deal1 {
		t.Fatalf("kept = %+v, want the uncited intent and the cited deal", kept)
	}
}

// The model cites by the ref the payload showed. Anything else — a ref never
// minted, the raw id, or a shown ref under another kind — names no record.
func TestACitationNamesARecordOnlyByItsShownRef(t *testing.T) {
	for entityID, want := range map[string]int{"d1": 1, "d2": 0, deal1: 0} {
		reasons := []modelReason{{Kind: "deal", Label: "renewal", EntityType: "deal", EntityID: entityID}}
		if kept := keepNarrow(reasons); len(kept) != want {
			t.Errorf("citing %q kept %+v, want %d reason(s)", entityID, kept, want)
		}
	}
	asActivity := []modelReason{{Kind: "deal", Label: "renewal", EntityType: "activity", EntityID: "d1"}}
	if kept := keepNarrow(asActivity); len(kept) != 0 {
		t.Errorf("a deal ref cited as an activity was kept as %+v", kept)
	}
}

// A null type beside an id is not "no citation": it is a pair naming a record
// by ref alone, and a ref reads back only under the kind it was shown as.
// Either way the reason is dropped, never kept as a chip pointing at a record
// nobody checked.
func TestACitationWithANullTypeIsDropped(t *testing.T) {
	for name, entityID := range map[string]string{
		"a ref this draft never showed": "d9",
		"a ref it did show":             "d1",
	} {
		var out modelDraft
		raw := `{"subject":"s","body":"b","reasoning":[{"kind":"deal","label":"signed the renewal",` +
			`"entity_type":null,"entity_id":"` + entityID + `"}]}`
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if kept := keepNarrow(out.Reasoning); len(kept) != 0 {
			t.Errorf("%s with a null type was kept as %+v", name, kept)
		}
	}
}
