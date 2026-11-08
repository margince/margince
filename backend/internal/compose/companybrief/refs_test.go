// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cited is a reply scripted with record ids as the model writes it: each id
// the request showed spelled as its ref. An id the request never showed stays
// as written, which is how a model that invents one answers.
func cited(in Input, reply string) string { return shownRefs(in).Refer(reply) }

// parseCitedBrief parses reply as written for in, the account in.ID names.
func parseCitedBrief(reply string, in Input) ([]Sentence, error) {
	return ParseBrief(cited(in, reply), in.ID, in)
}

func parseCitedSections(reply, companyID string, in Input) ([]Section, error) {
	return ParseBriefSections(cited(in, reply), companyID, in)
}

// citingLane answers every request with reply, its ids spelled as the refs
// in's request shows.
type citingLane struct {
	in    Input
	reply string
}

func (l citingLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: cited(l.in, l.reply)}, nil
}

// The prompts show every record of the account by its ref, the account
// itself included. A reply citing a ref reads back to the record's id; the
// id itself, and a ref the request never minted, ground nothing.
func TestTheBriefShowsRefsAndReadsThemBackToRecords(t *testing.T) {
	in := inputFixture()
	shown := BriefRequest(in, "en").Messages[0].Content
	if claims.SpellsRecordID(shown) || !strings.Contains(shown, `"id":"d1"`) || !strings.Contains(shown, `"id":"co1"`) {
		t.Fatalf("the request must show refs and never an id:\n%s", shown)
	}
	deal := in.OpenDeals[0].ID
	for ref, want := range map[string]int{"d1": 1, deal: 0, "d2": 0} {
		kept, err := ParseBrief(`{"sentences":[{"text":"The retrofit deal has stalled.","evidence":[{"entity_type":"deal","entity_id":"`+ref+`"}]}]}`, briefCompanyID, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != want || (want == 1 && kept[0].Evidence[0].EntityID != deal) {
			t.Errorf("citing %q kept %+v, want %d sentence(s) citing the deal's id", ref, kept, want)
		}
	}
}
