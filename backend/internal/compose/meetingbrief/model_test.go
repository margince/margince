// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What the model lane is allowed to produce, and what it is not.
//
// The prompt asks for the right thing; the filter is what makes it true. Every
// case here drives the real filter with a reply the model could plausibly send,
// because a lane trusted to obey its prompt is a lane with no guarantee at all.

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// laneReturning answers one canned reply, and records the request it was sent.
type laneReturning struct {
	reply string
	err   error
	sent  *model.Request
}

func (l *laneReturning) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.sent = &req
	if l.err != nil {
		return model.Response{}, l.err
	}
	return model.Response{Text: l.reply}, nil
}

func TestNoLaneIsTheDeterministicFloorRatherThanAnError(t *testing.T) {
	sections, by := Write(context.Background(), nil, fullInput(), string(textlang.English))
	if by != crmcontracts.Deterministic {
		t.Errorf("generated_by = %v, want deterministic for a deployment with no model", by)
	}
	if len(sections) == 0 {
		t.Fatal("a deployment with no model got no brief at all")
	}
}

func TestALaneThatFailsFallsBackAndSaysSo(t *testing.T) {
	lane := &laneReturning{err: errors.New("over budget")}
	sections, by := Write(context.Background(), lane, fullInput(), string(textlang.English))
	if by != crmcontracts.Deterministic {
		t.Errorf("generated_by = %v, want deterministic when the lane failed", by)
	}
	if len(sections) == 0 {
		t.Fatal("a failed model lane took the brief down with it")
	}
}

func TestARepliedBriefIsAttributedToTheModel(t *testing.T) {
	in := fullInput()
	lane := &laneReturning{reply: `{"sections":[{"kind":"goal","sentences":[
		{"text":"Get them to name a date.","nature":"recommendation",
		 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]}]}`}
	sections, by := Write(context.Background(), lane, in, string(textlang.English))
	if by != crmcontracts.Model {
		t.Fatalf("generated_by = %v, want model for a reply that survived the filter", by)
	}
	// The goal is the model's; the rest is the floor's, restored because the
	// reply did not answer those sections.
	goal := sectionOfKind(sections, crmcontracts.MeetingBriefSectionKindGoal)
	if len(goal.Sentences) != 1 || goal.Sentences[0].Text != "Get them to name a date." {
		t.Fatalf("the goal section = %+v, want the model's one line", goal.Sentences)
	}
}

func TestASentenceCitingARecordTheBriefNeverSawIsDropped(t *testing.T) {
	in := fullInput()
	lane := &laneReturning{reply: `{"sections":[{"kind":"goal","sentences":[
		{"text":"Invented.","nature":"fact",
		 "evidence":[{"entity_type":"deal","entity_id":"0198f000-0000-7000-8000-0000000000ff"}]},
		{"text":"Real.","nature":"fact",
		 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]}]}`}
	sections, by := Write(context.Background(), lane, in, string(textlang.English))
	if by != crmcontracts.Model {
		t.Fatalf("generated_by = %v, want model", by)
	}
	for _, line := range sections[0].Sentences {
		if line.Text == "Invented." {
			t.Fatal("a sentence citing a record this brief never carried was shown to the reader")
		}
	}
}

func TestAReplyThatCitesNothingRealFallsBackToTheFloor(t *testing.T) {
	lane := &laneReturning{reply: `{"sections":[{"kind":"goal","sentences":[
		{"text":"All invented.","nature":"fact",
		 "evidence":[{"entity_type":"deal","entity_id":"0198f000-0000-7000-8000-0000000000ff"}]}]}]}`}
	sections, by := Write(context.Background(), lane, fullInput(), string(textlang.English))
	if by != crmcontracts.Deterministic {
		t.Errorf("generated_by = %v, want the floor when nothing survived grounding", by)
	}
	if len(sections) == 0 {
		t.Fatal("the reader got no brief at all")
	}
}

func TestAJudgementIsRefusedInASectionThatMayOnlyStateFacts(t *testing.T) {
	in := fullInput()
	lane := &laneReturning{reply: `{"sections":[
		{"kind":"attendees","sentences":[{"text":"They are stalling.","nature":"assessment",
		 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]},
		{"kind":"risks","sentences":[{"text":"They are stalling.","nature":"assessment",
		 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]}]}`}
	sections, _ := Write(context.Background(), lane, in, string(textlang.English))
	// The section itself survives — the floor's own attendee lines are
	// restored when the model's are refused. What must not survive is the
	// model's judgement inside it.
	for _, section := range sections {
		if section.Kind != crmcontracts.MeetingBriefSectionKindAttendees {
			continue
		}
		for _, line := range section.Sentences {
			if line.Text == "They are stalling." {
				t.Error("a judgement was allowed into attendees, which may only state facts")
			}
		}
	}
}

func TestTheAdviceIsCappedAcrossTheWholeBriefNotPerSection(t *testing.T) {
	in := fullInput()
	one := `{"text":"Do a thing.","nature":"recommendation","evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}`
	lane := &laneReturning{reply: `{"sections":[
		{"kind":"goal","sentences":[` + one + `,` + one + `]},
		{"kind":"talking_points","sentences":[` + one + `,` + one + `,` + one + `]}]}`}
	sections, _ := Write(context.Background(), lane, in, string(textlang.English))
	advice := 0
	for _, section := range sections {
		for _, line := range section.Sentences {
			if line.Nature == natureRecommendation {
				advice++
			}
		}
	}
	if advice > maxRecommendations {
		t.Errorf("the brief carries %d recommendations, want at most %d", advice, maxRecommendations)
	}
}

func TestTheRequestFencesTheMeetingAndNamesTheLanguageItWasGiven(t *testing.T) {
	in := fullInput()
	req := BriefRequest(in, string(textlang.German))
	if !strings.Contains(req.Messages[0].Content, in.Subject) {
		t.Error("the request does not carry the meeting it is about")
	}
	if strings.Contains(req.System, in.Subject) {
		t.Error("the meeting's own text reached the SYSTEM prompt, outside the fence")
	}
	// The language this call was GIVEN, not merely the word "language". The
	// previous spelling looked for that word in the system prompt, which the
	// surrounding prose contains whatever language is passed — it passed for
	// years over a prompt that pointed the model at a summary field that was
	// never present, because json.Marshal emitted it as "Language".
	//
	// German rather than English, so a builder that ignored its argument and
	// hard-coded the default would fail here rather than pass.
	if !strings.Contains(req.System, "German") {
		t.Errorf("the prompt was built for German and does not ask for it:\n%s", req.System)
	}
}

// A model that answers one section must not silently take the others off the
// page. It may change how the brief READS; it may not change what it COVERS.
func TestASparseReplyKeepsTheFloorsCoverage(t *testing.T) {
	in := fullInput()
	floor := Deterministic(in)
	lane := &laneReturning{reply: `{"sections":[{"kind":"goal","sentences":[
		{"text":"Get them to name a date.","nature":"recommendation",
		 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]}]}`}
	sections, by := Write(context.Background(), lane, in, string(textlang.English))
	if by != crmcontracts.Model {
		t.Fatalf("generated_by = %v, want model", by)
	}
	for _, want := range floor {
		if len(want.Sentences) == 0 {
			continue
		}
		found := false
		for _, got := range sections {
			if got.Kind == want.Kind && len(got.Sentences) > 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("the model's sparse reply dropped the %q section the floor had", want.Kind)
		}
	}
}
