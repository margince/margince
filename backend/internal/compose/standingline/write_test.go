// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	acceptedID = "01a215b6-40a5-717c-897c-768eb6506be2"
	offerID    = "01a211e5-134b-7a4c-8b7a-686837ef7f21"
)

func offerInput() Input {
	return Input{Kind: KindContact, Previous: "Sebastian asked for a revised offer.", Recent: []Act{
		{
			ID: acceptedID, Kind: "email", At: "2026-11-25", Author: "Sebastian Sachsinger", Side: sideCustomer,
			Subject: "Re: Revised offer", Text: "The revised offer works for us. Please send the contract draft by Friday.",
		},
		{
			ID: offerID, Kind: "email", At: "2026-11-23", Author: "Lena Hoffmann", Side: sideOurs,
			Subject: "Revised offer", Text: "Attached is the revised offer.",
		},
	}}
}

// reply is a scripted model answer, each id spelled as the ref the request showed.
func reply(t *testing.T, in Input, keep bool, sentences ...claims.Sentence) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"keep_previous": keep, "sentences": sentences})
	if err != nil {
		t.Fatal(err)
	}
	return shownRefs(in).Show(string(encoded))
}

func cited(text string, activities ...string) claims.Sentence {
	s := claims.Sentence{Text: text}
	for _, id := range activities {
		s.Evidence = append(s.Evidence, claims.Evidence{EntityType: citeActivity, EntityID: id})
	}
	return s
}

func TestALineSaysWhatTheNewestMailSaid(t *testing.T) {
	in := offerInput()
	want := "In his mail on 25 November, Sebastian Sachsinger accepted the revised offer and asked for the contract draft by Friday."
	got, err := ParseLine(reply(t, in, false, cited(want, acceptedID)), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Keep || got.Text != want {
		t.Fatalf("answer = %+v, want the new line", got)
	}
	if len(got.Cited) != 1 || got.Cited[0] != acceptedID {
		t.Fatalf("the line cites %v, want only the mail it rests on", got.Cited)
	}
}

func TestAStockLineIsRefused(t *testing.T) {
	in := offerInput()
	got, err := ParseLine(reply(t, in, false, cited("Both sides have written.", acceptedID, offerID)), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "" {
		t.Fatalf("kept %q, want no line from stock copy", got.Text)
	}
}

func TestALineCitingNothingShownIsRefused(t *testing.T) {
	in := offerInput()
	got, err := ParseLine(reply(t, in, false, cited("Sebastian signed the contract.", "01a2130a-0000-7000-8000-000000000001")), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "" {
		t.Fatalf("kept %q, want no line resting on an activity the model was not shown", got.Text)
	}
}

func TestAHoldingLineIsKept(t *testing.T) {
	in := offerInput()
	got, err := ParseLine(reply(t, in, true), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Keep {
		t.Fatalf("answer = %+v, want the shown line kept", got)
	}
	in.Previous = ""
	if got, _ := ParseLine(reply(t, in, true), in); got.Keep {
		t.Fatal("kept a line that was never written")
	}
}

func TestAWithheldActivityIsNeverRead(t *testing.T) {
	withheld := crmcontracts.ActivityContentStateWithheld
	secret := "the board said no"
	rows := []crmcontracts.Activity{
		{Id: openapi_types.UUID(ids.MustParse(acceptedID)), Kind: crmcontracts.ActivityKindEmail, ContentState: &withheld, Body: &secret},
		{Id: openapi_types.UUID(ids.MustParse(offerID)), Kind: crmcontracts.ActivityKindNote},
	}
	in := FromActivities(KindContact, "", rows)
	if len(in.Recent) != 1 || in.Recent[0].ID != offerID {
		t.Fatalf("recent = %+v, want only the activity whose content may be read", in.Recent)
	}
}

func TestTheFingerprintIgnoresTheShownLine(t *testing.T) {
	in := offerInput()
	first, err := Fingerprint(in, "r1", "en")
	if err != nil {
		t.Fatal(err)
	}
	in.Previous = "A line written since."
	second, _ := Fingerprint(in, "r1", "en")
	if first != second {
		t.Fatal("a new line changed the fingerprint of the same activities, so each write would ask again")
	}
	in.Recent = in.Recent[1:]
	if third, _ := Fingerprint(in, "r1", "en"); third == first {
		t.Fatal("other activities read as the same input")
	}
}

// scriptedLane answers every request with one fixed reply.
type scriptedLane struct {
	reply string
	asked model.Request
}

func (s *scriptedLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.asked = req
	return model.Response{Text: s.reply}, nil
}

func TestTheModelIsShownTheNewestMailAndOnlyThoseActivities(t *testing.T) {
	in := offerInput()
	want := "Sebastian Sachsinger accepted the revised offer and asked for the contract draft by Friday."
	lane := &scriptedLane{reply: reply(t, in, false, cited(want, acceptedID))}
	got, err := Write(context.Background(), lane, in, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != want {
		t.Fatalf("Write = %+v, want the model's line", got)
	}
	if !strings.Contains(lane.asked.Messages[0].Content, "contract draft by Friday") {
		t.Error("the request never showed the newest mail's words")
	}
	if lane.asked.Subjects == nil || len(lane.asked.Subjects.Activities) != len(in.Recent) {
		t.Errorf("the request names %+v as its subjects, want the %d activities shown", lane.asked.Subjects, len(in.Recent))
	}
}

func TestNoLineIsAskedForWithoutALaneOrAnActivity(t *testing.T) {
	if _, err := Write(context.Background(), nil, offerInput(), "en"); err == nil {
		t.Error("wrote a line with no model lane")
	}
	empty := offerInput()
	empty.Recent = nil
	if _, err := Write(context.Background(), &scriptedLane{}, empty, "en"); err == nil {
		t.Error("asked for a line about a record with nothing to read")
	}
}
