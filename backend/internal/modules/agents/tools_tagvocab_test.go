// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The vocabulary verbs, asserted at the tool rather than at the store: what a
// model sends has to arrive at the seam as the caller meant it. The authority
// half — that coining needs `tag.create`, which the seeded roles give Admin
// and Ops alone — belongs to the store and is proved in the integration lane;
// what these hold is that the tool does not garble the ask on the way there.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestCreateTagPassesTheNameAndColourThrough(t *testing.T) {
	t.Parallel()
	var created createTagArgs
	out, err := (createTag{tags: stubTags{created: &created}}).Handle(
		context.Background(),
		json.RawMessage(`{"name":"K5 Summit","color":"amber"}`))
	if err != nil {
		t.Fatalf("creating answered %v, want the word coined", err)
	}
	if created.name != "K5 Summit" {
		t.Errorf("the seam was asked for %q, want the name as typed", created.name)
	}
	if created.color == nil || *created.color != "amber" {
		t.Errorf("the seam got colour %v, want amber", created.color)
	}
	var word Tag
	if err := json.Unmarshal(out, &word); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if word.Name != "K5 Summit" {
		t.Errorf("the answer names %q, want the word that was coined", word.Name)
	}
}

// A colour is optional, and omitting it must not become a request to clear
// one: the store distinguishes "the caller said nothing" from "the caller
// asked for none", and a tool that collapsed them would silently strip the
// colour off every word it touched.
func TestCreateTagWithoutAColourAsksForNone(t *testing.T) {
	t.Parallel()
	var created createTagArgs
	if _, err := (createTag{tags: stubTags{created: &created}}).Handle(
		context.Background(), json.RawMessage(`{"name":"Parked"}`)); err != nil {
		t.Fatalf("creating answered %v, want the word coined", err)
	}
	if created.color != nil {
		t.Errorf("the seam got colour %q, want nothing said about it", *created.color)
	}
}

// An omitted field is left alone. This is the whole reason update takes
// pointers: a tool that sent zero values would rename every tag it recoloured
// to the empty string.
func TestUpdateTagLeavesUnnamedFieldsAlone(t *testing.T) {
	t.Parallel()
	var edited editTagArgs
	id := ids.NewV7()
	// Only the NAME is sent, so nothing else may reach the seam. A payload
	// that mentioned colour could not tell "left alone" from "named": the
	// omitted field is the whole subject here.
	if _, err := (updateTag{tags: stubTags{edited: &edited}}).Handle(
		context.Background(),
		json.RawMessage(`{"tag_id":"`+id.String()+`","name":"Key Accounts"}`)); err != nil {
		t.Fatalf("updating answered %v, want the rename", err)
	}
	if edited.tagID != id {
		t.Errorf("the seam was asked about %v, want %v", edited.tagID, id)
	}
	if edited.edit.Name == nil || *edited.edit.Name != "Key Accounts" {
		t.Errorf("the edit's name is %v, want the one that was sent", edited.edit.Name)
	}
	if edited.edit.Color != nil {
		t.Error("the edit names a colour the caller never sent, so the store would clear or set one")
	}
	if edited.edit.Description != nil {
		t.Error("the edit names a description the caller never sent")
	}
}

// Clearing is spelled as a VALUE, the way UpdateTagRequest spells it: "none"
// removes the colour and "" removes the text. The store reads that as a
// non-nil outer pointer holding a nil inner one — named, and asked to be
// empty — which is what tells it apart from a field nobody mentioned.
func TestUpdateTagClearsAColourAskedToBeNone(t *testing.T) {
	t.Parallel()
	var edited editTagArgs
	if _, err := (updateTag{tags: stubTags{edited: &edited}}).Handle(
		context.Background(),
		json.RawMessage(`{"tag_id":"`+ids.NewV7().String()+`","color":"none","description":""}`)); err != nil {
		t.Fatalf("updating answered %v, want the clear", err)
	}
	if edited.edit.Color == nil {
		t.Fatal("the colour was not named at all, so the store leaves it alone — the clear was lost")
	}
	if *edited.edit.Color != nil {
		t.Errorf("the colour was set to %q, want it cleared", **edited.edit.Color)
	}
	if edited.edit.Description == nil {
		t.Fatal("the description was not named, so the store leaves it alone")
	}
	if *edited.edit.Description != nil {
		t.Errorf("the description was set to %q, want it cleared", **edited.edit.Description)
	}
}

// The schema a model reads has to offer the same colours the REST body does,
// including the one that clears. A tool refusing a value the HTTP door accepts
// is the two transports disagreeing about one behaviour.
func TestUpdateTagOffersTheClearingValue(t *testing.T) {
	t.Parallel()
	spec := (updateTag{tags: stubTags{}}).Spec()
	var schema struct {
		Properties struct {
			Color struct {
				Enum []string `json:"enum"`
			} `json:"color"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatalf("the input schema does not decode: %v", err)
	}
	var offersNone bool
	for _, value := range schema.Properties.Color.Enum {
		if value == clearColor {
			offersNone = true
		}
	}
	if !offersNone {
		t.Errorf("the colour enum is %v and does not offer %q, so a model cannot clear a colour the REST body lets it clear",
			schema.Properties.Color.Enum, clearColor)
	}
}

// A write answers the WORD, not a counted detail. TagDetail declares people,
// companies and deals as required fields, and a write has counted none of
// them — shipping it would report a tag on fifty records as carried by none.
func TestAVocabularyWriteAnswersTheWordWithoutInventingCounts(t *testing.T) {
	t.Parallel()
	for name, spec := range map[string]mcp.ToolSpec{
		"create_tag": (createTag{tags: stubTags{}}).Spec(),
		"update_tag": (updateTag{tags: stubTags{}}).Spec(),
	} {
		var shape struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.OutputSchema, &shape); err != nil {
			t.Fatalf("%s: the output schema does not decode: %v", name, err)
		}
		for _, counted := range []string{"people", "companies", "deals"} {
			if _, present := shape.Properties[counted]; present {
				t.Errorf("%s answers a %q count it never took", name, counted)
			}
		}
		if _, present := shape.Properties["tag_id"]; !present {
			t.Errorf("%s does not answer the tag's id", name)
		}
	}
}
