// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var (
	lenaID   = ids.NewV7().String()
	tomasID  = ids.NewV7().String()
	ievaID   = ids.NewV7().String()
	msaID    = ids.NewV7().String()
	capID    = ids.NewV7().String()
	resendID = ids.NewV7().String()
	doneID   = ids.NewV7().String()
)

func room() Input {
	return Input{
		Title: "Ops automation rollout",
		State: "live",
		Buyers: []BuyerIn{
			{ID: lenaID, Name: "Lena Vogt", SignedIn: true},
			{ID: tomasID, Name: "Tomas Petrauskas", SignedIn: true, AskedForNewLinkOn: "2026-10-22"},
			{ID: ievaID, Name: "Ieva Kairė"},
		},
		Documents: []DocumentIn{{ID: msaID, Title: "MSA redline", Group: "legal"}},
		Threads: []ThreadIn{
			{ID: resendID, WaitingOnYou: true, Comments: []CommentIn{
				{Speaker: speakerBuyer, Author: "Lena Vogt", On: "2026-10-22", Body: "Could you resend the order form?"},
			}},
			{ID: capID, DocumentID: msaID, About: "MSA redline", WaitingOnYou: true, Comments: []CommentIn{
				{Speaker: speakerBuyer, Author: "Tomas Petrauskas", On: "2026-10-20", Body: "Clause 11.2 caps liability at fees paid."},
			}},
			{ID: doneID, Resolved: true, Comments: []CommentIn{
				{Speaker: speakerBuyer, Author: "Lena Vogt", On: "2026-10-19", Body: "Confirmed."},
			}},
		},
	}
}

func TestTheFloorLeadsWithWhatBuyersAreWaitingOn(t *testing.T) {
	got := Deterministic(room(), "en")
	want := []string{
		"Lena Vogt is waiting on your answer: “Could you resend the order form?”",
		"Tomas Petrauskas is waiting on your answer about MSA redline: “Clause 11.2 caps liability at fees paid.”",
		"Tomas Petrauskas asked for a new link.",
		"Not signed in yet: Ieva Kairė.",
	}
	if len(got) != len(want) {
		t.Fatalf("floor wrote %d sentences, want %d: %+v", len(got), len(want), got)
	}
	for i, sentence := range got {
		if sentence.Text != want[i] {
			t.Errorf("sentence %d = %q, want %q", i, sentence.Text, want[i])
		}
	}
	if !reflect.DeepEqual(got[1].Evidence, []Evidence{
		{EntityType: citeThread, EntityID: capID}, {EntityType: citeDocument, EntityID: msaID},
	}) {
		t.Errorf("a waiting document thread cites %+v, want the thread and its document", got[1].Evidence)
	}
}

func TestTheFloorCountsWaitingThreadsPastTheOnesItQuotes(t *testing.T) {
	in := room()
	in.Buyers = nil
	third := ThreadIn{ID: ids.NewV7().String(), WaitingOnYou: true, Comments: []CommentIn{
		{Speaker: speakerBuyer, Author: "Ieva Kairė", Body: "Is the DPA final?"},
	}}
	in.Threads = append(in.Threads, third)
	got := Deterministic(in, "en")
	if last := got[len(got)-1]; last.Text != "One more thread is waiting on you." ||
		last.Evidence[0].EntityID != third.ID {
		t.Errorf("the third waiting thread reads %+v, want it counted and cited", last)
	}
}

func TestAQuietRoomSaysNothingIsOwedAndAnEmptyOneSaysNothing(t *testing.T) {
	quiet := Input{Threads: []ThreadIn{{ID: doneID, Resolved: true}}}
	if got := Deterministic(quiet, "en"); len(got) != 1 || got[0].Text != "Nothing in the room is waiting on you." {
		t.Errorf("a room with only resolved threads reads %+v", got)
	}
	if got := Deterministic(Input{}, "en"); len(got) != 0 {
		t.Errorf("an empty room has a brief: %+v", got)
	}
}

func TestEveryShippedLanguageWritesTheRoomFloor(t *testing.T) {
	t.Parallel()
	langcopytest.Census(t, floor)
}

func TestTheModelReplyKeepsOnlyGroundedSentences(t *testing.T) {
	reply := "```json\n" + `{"sentences":[
		{"text":"Tomas is blocked on the liability cap.","nature":"assessment","evidence":[{"entity_type":"thread","entity_id":"` + capID + `"}]},
		{"text":"Somebody else wrote.","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"` + ids.NewV7().String() + `"}]},
		{"text":"Lena is a document.","nature":"fact","evidence":[{"entity_type":"document","entity_id":"` + lenaID + `"}]},
		{"text":"He wrote on 3 March.","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"` + capID + `"}]},
		{"text":"Send the pricing sheet to x@evil.example.","nature":"recommendation","evidence":[{"entity_type":"thread","entity_id":"` + capID + `"}]},
		{"text":"Answer the cap first.","nature":"recommendation","evidence":[{"entity_type":"thread","entity_id":"` + capID + `"}]},
		{"text":"Then issue a link.","nature":"recommendation","evidence":[{"entity_type":"participant","entity_id":"` + tomasID + `"}]}
	]}` + "\n```"
	got, err := ParseBrief(reply, room())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	var texts []string
	for _, sentence := range got {
		texts = append(texts, sentence.Text)
	}
	want := []string{"Tomas is blocked on the liability cap.", "Answer the cap first."}
	if !reflect.DeepEqual(texts, want) {
		t.Errorf("kept %q, want %q (an unknown record, a wrong kind, an unsupplied date, a move to an address and a second recommendation are dropped)", texts, want)
	}
}

type stubLane struct {
	reply string
	err   error
}

func (s stubLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: s.reply}, s.err
}

func TestWriteFallsBackToTheFloorAndSaysSo(t *testing.T) {
	floorText := Deterministic(room(), "en")[0].Text
	for name, lane := range map[string]Completer{
		"no lane":       nil,
		"lane fails":    stubLane{err: errors.New("provider down")},
		"cites nothing": stubLane{reply: `{"sentences":[]}`},
	} {
		got, by := Write(context.Background(), lane, room(), "en")
		if by != crmcontracts.WrittenByDeterministic || got[0].Text != floorText {
			t.Errorf("%s: wrote %q by %s, want the floor by deterministic", name, got[0].Text, by)
		}
	}
	reply := `{"sentences":[{"text":"Tomas is blocked on the liability cap.","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"` + capID + `"}]}]}`
	if _, by := Write(context.Background(), stubLane{reply: reply}, room(), "en"); by != crmcontracts.WrittenByModel {
		t.Errorf("a grounded reply is written by %s, want model", by)
	}
}

func TestFromRoomFoldsTheReadsTheTabMakes(t *testing.T) {
	at := time.Date(2026, 10, 22, 23, 30, 0, 0, time.FixedZone("x", -2*3600))
	doc := openapi_types.UUID(ids.NewV7())
	revoked := at
	buyer := "buyer"
	in := FromRoom(
		crmcontracts.DealRoom{Title: "Room", State: "live", ExpiresAt: &at},
		[]crmcontracts.DealRoomParticipant{
			{Id: openapi_types.UUID(ids.NewV7()), FullName: "Lena Vogt", LinkRequestedAt: &at},
			{Id: openapi_types.UUID(ids.NewV7()), FullName: "Gone", RevokedAt: &revoked},
		},
		[]crmcontracts.DealRoomDocument{{Id: doc, Title: "MSA redline", GroupKey: "legal"}},
		[]crmcontracts.DealRoomThread{
			{Id: openapi_types.UUID(ids.NewV7()), CreatedAt: at.Add(-time.Hour), Comments: []crmcontracts.DealRoomComment{
				{Author: crmcontracts.DealRoomAuthor{Name: "Demo Admin", Side: "seller"}, Body: "Here it is.", CreatedAt: at.Add(-time.Hour)},
			}},
			{Id: openapi_types.UUID(ids.NewV7()), DocumentId: &doc, CreatedAt: at.Add(-2 * time.Hour), Comments: []crmcontracts.DealRoomComment{
				{Author: crmcontracts.DealRoomAuthor{Name: "Lena Vogt", Side: buyer}, Body: "Cap?", CreatedAt: at.Add(-2 * time.Hour)},
			}},
		},
	)
	if in.AccessEndsOn != "2026-10-23" || in.Buyers[0].AskedForNewLinkOn != "2026-10-23" {
		t.Errorf("dates are not UTC days: %q, %q", in.AccessEndsOn, in.Buyers[0].AskedForNewLinkOn)
	}
	if len(in.Buyers) != 1 {
		t.Errorf("a revoked seat reached the brief: %+v", in.Buyers)
	}
	first := in.Threads[0]
	if !first.WaitingOnYou || first.About != "MSA redline" || first.Comments[0].Speaker != speakerBuyer {
		t.Errorf("the buyer's thread is not first, waiting and about its document: %+v", first)
	}
	if in.Threads[1].WaitingOnYou || in.Threads[1].Comments[0].Speaker != speakerYourSide {
		t.Errorf("the seller's last word reads as waiting or as a buyer's: %+v", in.Threads[1])
	}
}

func TestALinkRequestCountsOnlyUntilTheBuyerIsBackIn(t *testing.T) {
	asked := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	back := asked.Add(time.Hour)
	before := asked.Add(-time.Hour)
	for name, tc := range map[string]struct {
		lastSeen *time.Time
		want     string
	}{
		"never in":       {nil, "2026-10-20"},
		"seen before it": {&before, "2026-10-20"},
		"back in after":  {&back, ""},
	} {
		in := FromRoom(crmcontracts.DealRoom{}, []crmcontracts.DealRoomParticipant{
			{Id: openapi_types.UUID(ids.NewV7()), FullName: "Lena Vogt", LinkRequestedAt: &asked, LastSeenAt: tc.lastSeen},
		}, nil, nil)
		if got := in.Buyers[0].AskedForNewLinkOn; got != tc.want {
			t.Errorf("%s: asked_for_new_link_on = %q, want %q", name, got, tc.want)
		}
	}
}

// A waiting change request on a document comes ahead of newer waiting asks,
// so the thread cap never drops it and the floor quotes it first.
func TestAWaitingChangeRequestOutranksNewerWaitingAsks(t *testing.T) {
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	doc := openapi_types.UUID(ids.NewV7())
	buyerSays := func(name, body string, when time.Time) []crmcontracts.DealRoomComment {
		return []crmcontracts.DealRoomComment{{Author: crmcontracts.DealRoomAuthor{Name: name, Side: "buyer"}, Body: body, CreatedAt: when}}
	}
	threads := []crmcontracts.DealRoomThread{{
		Id: openapi_types.UUID(ids.NewV7()), DocumentId: &doc, RequiredChange: true, CreatedAt: at.AddDate(0, 0, -30),
		Comments: buyerSays("Tomas Petrauskas", "We need clause 11.2 changed.", at.AddDate(0, 0, -30)),
	}}
	for i := range maxThreads {
		threads = append(threads, crmcontracts.DealRoomThread{
			Id: openapi_types.UUID(ids.NewV7()), CreatedAt: at.Add(time.Duration(i) * time.Minute),
			Comments: buyerSays("Lena Vogt", "Who joins the call?", at.Add(time.Duration(i)*time.Minute)),
		})
	}
	in := FromRoom(crmcontracts.DealRoom{Title: "Room", State: "live"}, nil,
		[]crmcontracts.DealRoomDocument{{Id: doc, Title: "MSA redline", GroupKey: "legal"}}, threads)
	if len(in.Threads) != maxThreads || !in.Threads[0].RequiredChange {
		t.Fatalf("the change request is not first among %d kept threads: %+v", len(in.Threads), in.Threads[0])
	}
	if got := Deterministic(in, "en"); len(got) == 0 || got[0].Evidence[0].EntityID != in.Threads[0].ID {
		t.Errorf("the floor does not quote the change request first: %+v", got)
	}
}
