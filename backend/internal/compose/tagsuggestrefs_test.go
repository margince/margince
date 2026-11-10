// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/tagsuggest"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func tagSuggestRefsBatch() ([]collections.SuggestibleTag, []tagsuggest.Item) {
	tags := []collections.SuggestibleTag{
		{ID: ids.New[ids.TagKind](), Name: "Ledger migration", Description: "moves bookkeeping off spreadsheets"},
		{ID: ids.New[ids.TagKind](), Name: "Premium support", Description: "wants faster help"},
	}
	items := []tagsuggest.Item{
		{ID: ids.NewV7(), Kind: "note", Subject: "Month-end", Body: "We close the month in Excel."},
		{ID: ids.NewV7(), Kind: "note", Subject: "Lunch", Body: "Thanks for lunch."},
	}
	return tags, items
}

func TestTheTagPromptNamesItemsAndTagsByRefAndTheReplyReadsBack(t *testing.T) {
	tags, items := tagSuggestRefsBatch()
	req := tagSuggestRequest(tags, items)
	for _, id := range []string{tags[0].ID.String(), items[0].ID.String()} {
		if strings.Contains(req.Messages[0].Content+string(req.ResponseSchema), id) {
			t.Fatalf("the request shows the record id %s", id)
		}
	}
	answers, err := parseTagSuggestReply(`{"items":[{"id":"m1","tags":["t1"]},{"id":"m2","tags":[]}]}`, tags, items)
	if err != nil {
		t.Fatal(err)
	}
	if answers[0].ID != items[0].ID.String() || !slices.Equal(answers[0].Tags, []string{tags[0].ID.String()}) {
		t.Errorf("m1/t1 read back as %+v, want the first item and the first tag", answers[0])
	}
}

func TestATagReplyNamingARefNeverShownOrARawIDIsRefused(t *testing.T) {
	tags, items := tagSuggestRefsBatch()
	for name, reply := range map[string]string{
		"an unshown tag ref":  `{"items":[{"id":"m1","tags":["t9"]},{"id":"m2","tags":[]}]}`,
		"a raw tag id":        `{"items":[{"id":"m1","tags":["` + tags[0].ID.String() + `"]},{"id":"m2","tags":[]}]}`,
		"an unshown item ref": `{"items":[{"id":"m1","tags":[]},{"id":"m3","tags":[]}]}`,
		"a raw item id":       `{"items":[{"id":"m1","tags":[]},{"id":"` + items[1].ID.String() + `","tags":[]}]}`,
	} {
		if _, err := parseTagSuggestReply(reply, tags, items); err == nil {
			t.Errorf("%s: the reply was accepted", name)
		}
	}
}
