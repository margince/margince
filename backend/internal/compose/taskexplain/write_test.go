// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	taskID    = "01a215b6-40a5-717c-897c-768eb6506be2"
	sourceID  = "01a211e5-134b-7a4c-8b7a-686837ef7f21"
	parkingID = "01a2130a-0000-7000-8000-000000000001"
	contactID = "01a153a7-4665-7213-b213-5c821e9bfdb5"
)

func promiseInput() Input {
	source := contactbrief.ActIn{
		ID: sourceID, Kind: "email", At: "2026-11-25T08:47:42Z",
		Subject: "Re: Handover document and acceptance",
		Preview: "Once the shift leads have signed off on the rota export I will send the signed acceptance.",
		Speaker: &contactbrief.Speaker{Kind: contactbrief.SpeakerThem, Name: "Jonas Keller"},
	}
	parking := contactbrief.ActIn{
		ID: parkingID, Kind: "email", At: "2026-11-26T07:00:00Z",
		Subject: "Parking on Friday", Preview: "Park behind building B.",
		Speaker: &contactbrief.Speaker{Kind: contactbrief.SpeakerThem, Name: "Jonas Keller"},
	}
	return Input{
		Task:   TaskIn{ID: taskID, Type: "reach_out", Title: "Follow up on the acceptance", Origin: "commitment", Created: "2026-11-26"},
		Source: &source,
		Relationship: &contactbrief.Input{
			ID: contactID, Name: "Jonas Keller", Recent: []contactbrief.ActIn{parking, source},
		},
	}
}

// reply is a scripted model answer, each id spelled as the ref the request showed.
func reply(t *testing.T, in Input, sentences ...claims.Sentence) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"sentences": sentences})
	if err != nil {
		t.Fatal(err)
	}
	return shownRefs(in).Show(string(encoded))
}

func sentence(nature, text string, cited ...string) claims.Sentence {
	s := claims.Sentence{Text: text, Nature: nature}
	for _, id := range cited {
		s.Evidence = append(s.Evidence, claims.Evidence{EntityType: citeActivity, EntityID: id})
	}
	return s
}

func TestAnExplanationKeepsItsThreeParts(t *testing.T) {
	in := promiseInput()
	got, err := ParseExplanation(reply(t, in,
		sentence(natureFact, "Jonas Keller promised the signed acceptance once his shift leads sign off on the rota export.", sourceID),
		sentence(natureAssessment, "Without it the project cannot close.", sourceID),
		sentence(natureRecommendation, "Ask Jonas whether the shift leads have signed off on the rota export.", sourceID),
	), in)
	if err != nil {
		t.Fatal(err)
	}
	natures := make([]string, 0, len(got))
	for _, s := range got {
		natures = append(natures, s.Nature)
		if s.Evidence[0].EntityID != sourceID {
			t.Fatalf("a citation reads back as %q, want the source id", s.Evidence[0].EntityID)
		}
	}
	if strings.Join(natures, ",") != "fact,assessment,recommendation" {
		t.Fatalf("natures = %v, want fact, assessment, recommendation", natures)
	}
}

func TestAStockSentenceIsDropped(t *testing.T) {
	in := promiseInput()
	got, err := ParseExplanation(reply(t, in,
		sentence(natureFact, "Jonas Keller promised the signed acceptance.", sourceID),
		sentence(natureAssessment, "The task with Jonas Keller stays undone.", taskID),
		sentence(natureRecommendation, "Finish the task with Jonas Keller and mark it done, or move it to another day.", taskID),
	), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Nature != natureFact {
		t.Fatalf("kept %v, want only the fact", got)
	}
}

func TestEveryFloorTemplateIsSpecific(t *testing.T) {
	for _, lang := range []textlang.Lang{textlang.English, textlang.German, textlang.Vietnamese} {
		for _, s := range Floor(promiseInput(), string(lang)) {
			if Generic(s.Text) {
				t.Errorf("%s floor sentence %q is stock copy", lang, s.Text)
			}
		}
	}
}

func TestTheFloorQuotesTheSourceAndTheNewerMessage(t *testing.T) {
	got := Floor(promiseInput(), string(textlang.English))
	if len(got) != 2 {
		t.Fatalf("floor = %v, want the source and the newer message", got)
	}
	if !strings.Contains(got[0].Text, "rota export") || got[0].Evidence[0].EntityID != sourceID {
		t.Errorf("first floor sentence %q does not quote its source", got[0].Text)
	}
	if !strings.Contains(got[1].Text, "building B") || got[1].Evidence[0].EntityID != parkingID {
		t.Errorf("second floor sentence %q is not the newer message", got[1].Text)
	}
}

func TestTheFloorSaysNothingCameInSinceTheSource(t *testing.T) {
	in := promiseInput()
	in.Relationship.Recent = in.Relationship.Recent[1:]
	got := Floor(in, string(textlang.English))
	if len(got) != 2 || got[1].Text != "Nothing newer has come in since." {
		t.Fatalf("floor = %v, want the source then that nothing came in since", got)
	}
}

func TestAWithheldSourceIsNotQuoted(t *testing.T) {
	in := promiseInput()
	in.Source = &contactbrief.ActIn{ID: sourceID, Kind: "email", At: in.Source.At, Withheld: true}
	in.Relationship.Recent = nil
	if got := Floor(in, string(textlang.English)); len(got) != 0 {
		t.Fatalf("floor = %v, want nothing from a message the reader may not read", got)
	}
}
