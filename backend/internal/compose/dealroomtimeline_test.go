// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What each Deal Room event becomes on the deal's timeline. The routing and
// the wording are a pure function of the envelope, so they are driven here
// without a database; the write itself is exercised by the integration lane.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/dealrooms"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/events"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// roomPayload is what the three Deal Room events put on the bus, plus the
// empty object the housekeeping cases carry: the helper takes the union rather
// than `any` so a test cannot hand it something no room ever emits.
type roomPayload interface {
	crmcontracts.PublicEventDealRoomCommentPosted |
		crmcontracts.PublicEventDealRoomDecisionRecorded |
		struct{}
}

func roomEnvelope[P roomPayload](t *testing.T, eventType string, payload P) events.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", eventType, err)
	}
	return events.Envelope{
		Type:    eventType,
		Entity:  events.EntityRef{Type: roomTimelineEntity, ID: ids.NewV7()},
		Payload: raw,
	}
}

// The note has to name the PERSON and the DOCUMENT, not their categories.
//
// The earlier version of this test asserted the words "buyer" and "document"
// appeared somewhere, which "The buyer replied in the Deal Room / About a
// document in the room" satisfies while telling a reader nothing. Asserting
// the actual name and the actual title is the difference between a test that
// describes the feature and one that would have caught its absence.
func TestABuyerCommentBecomesANoteThatNamesTheBuyerAndTheDocument(t *testing.T) {
	deal := ids.NewV7()
	doc := openapi_types.UUID(ids.NewV7())
	name := "Thorsten Sifferlien"
	title := "Rahmenvertrag v3"
	note, carried, err := roomNote(roomEnvelope(t, dealrooms.EventCommentPosted,
		crmcontracts.PublicEventDealRoomCommentPosted{
			DealId:        openapi_types.UUID(deal),
			ThreadId:      openapi_types.UUID(ids.NewV7()),
			CommentId:     openapi_types.UUID(ids.NewV7()),
			DocumentId:    &doc,
			Side:          "buyer",
			OpensThread:   true,
			AuthorName:    &name,
			DocumentTitle: &title,
		}))
	if err != nil {
		t.Fatalf("routing a buyer comment: %v", err)
	}
	if !carried {
		t.Fatal("a buyer comment produced no note, so the deal's timeline never learns the buyer spoke")
	}
	if note.deal != deal {
		t.Errorf("note is filed against %s, want the deal %s the room belongs to", note.deal, deal)
	}
	if !strings.Contains(note.subject, name) {
		t.Errorf("subject %q does not NAME who spoke", note.subject)
	}
	if !strings.Contains(note.subject, "asked a question") {
		t.Errorf("subject %q does not say a thread was opened", note.subject)
	}
	if !strings.Contains(note.body, title) {
		t.Errorf("body %q does not NAME the document the comment was about", note.body)
	}
}

// An event minted before the payload carried a name still has to produce a
// true note. It falls back to the side — vaguer, never wrong, and never a
// placeholder name that would read as a real person.
func TestACommentWithoutANameFallsBackToTheSide(t *testing.T) {
	doc := openapi_types.UUID(ids.NewV7())
	note, carried, err := roomNote(roomEnvelope(t, dealrooms.EventCommentPosted,
		crmcontracts.PublicEventDealRoomCommentPosted{
			DealId:      openapi_types.UUID(ids.NewV7()),
			ThreadId:    openapi_types.UUID(ids.NewV7()),
			CommentId:   openapi_types.UUID(ids.NewV7()),
			DocumentId:  &doc,
			Side:        "buyer",
			OpensThread: false,
		}))
	if err != nil || !carried {
		t.Fatalf("routing an old-shape comment: carried=%v err=%v", carried, err)
	}
	if !strings.Contains(note.subject, "buyer") {
		t.Errorf("subject %q names neither a person nor a side", note.subject)
	}
	for _, bogus := range []string{"Unknown", "<nil>", "null"} {
		if strings.Contains(note.subject, bogus) || strings.Contains(note.body, bogus) {
			t.Errorf("note invents %q where the event carried nothing: %q / %q", bogus, note.subject, note.body)
		}
	}
}

func TestASellerReplyOnTheRoomIsNotReportedAsTheBuyerOpeningAThread(t *testing.T) {
	note, carried, err := roomNote(roomEnvelope(t, dealrooms.EventCommentPosted,
		crmcontracts.PublicEventDealRoomCommentPosted{
			DealId:      openapi_types.UUID(ids.NewV7()),
			ThreadId:    openapi_types.UUID(ids.NewV7()),
			CommentId:   openapi_types.UUID(ids.NewV7()),
			Side:        "seller",
			OpensThread: false,
		}))
	if err != nil || !carried {
		t.Fatalf("routing a seller reply: carried=%v err=%v", carried, err)
	}
	if strings.Contains(note.subject, "buyer") {
		t.Errorf("subject %q calls the seller a buyer", note.subject)
	}
	if strings.Contains(note.subject, "started") {
		t.Errorf("subject %q reports a reply as a new thread", note.subject)
	}
	if !strings.Contains(note.body, "room as a whole") {
		t.Errorf("body %q does not say the comment was about the room", note.body)
	}
}

func TestTheTwoBuyerDecisionsReadDifferently(t *testing.T) {
	for _, tc := range []struct{ kind, want, avoid string }{
		{"confirm_version", "confirmed", "asked for changes"},
		{"request_changes", "asked for changes", "confirmed"},
	} {
		note, carried, err := roomNote(roomEnvelope(t, dealrooms.EventDecisionRecorded,
			crmcontracts.PublicEventDealRoomDecisionRecorded{
				DealId:       openapi_types.UUID(ids.NewV7()),
				DecisionId:   openapi_types.UUID(ids.NewV7()),
				DocumentId:   openapi_types.UUID(ids.NewV7()),
				AttachmentId: openapi_types.UUID(ids.NewV7()),
				Kind:         tc.kind,
			}))
		if err != nil || !carried {
			t.Fatalf("routing %s: carried=%v err=%v", tc.kind, carried, err)
		}
		if !strings.Contains(note.subject, tc.want) {
			t.Errorf("%s reads %q, want it to say %q", tc.kind, note.subject, tc.want)
		}
		if strings.Contains(note.subject, tc.avoid) {
			t.Errorf("%s reads %q, which says the opposite decision", tc.kind, note.subject)
		}
	}
}

func TestTheSellersOwnHousekeepingStaysOffTheTimeline(t *testing.T) {
	// Opening, pausing and renaming a room are the seller's own acts on their
	// own tool. A note for each would crowd out the entries a reader of the
	// deal actually needs.
	for _, eventType := range []string{
		"deal_room.opened", "deal_room.updated", "deal_room.paused",
		"deal_room.published",
		"deal_room.resumed", "deal_room.closed", "deal_room.archived",
		"deal_room.participant_invited", "deal_room.thread_resolved",
	} {
		note, carried, err := roomNote(roomEnvelope(t, eventType, struct{}{}))
		if err != nil {
			t.Errorf("%s: %v", eventType, err)
		}
		if carried {
			t.Errorf("%s wrote a timeline note %q, which this consumer does not carry",
				eventType, note.subject)
		}
	}
}

func TestAPayloadNamingNoDealIsRefusedRatherThanFiledAgainstNothing(t *testing.T) {
	// Every room event carries its deal. One that does not would otherwise
	// decode to the zero UUID and write a note attached to nothing, which reads
	// exactly like a room nobody used.
	_, _, err := roomNote(roomEnvelope(t, dealrooms.EventDecisionRecorded,
		crmcontracts.PublicEventDealRoomDecisionRecorded{Kind: "confirm_version"}))
	if !errors.Is(err, dealrooms.ErrEventNamesNoDeal) {
		t.Fatalf("a decision naming no deal answered %v, want ErrEventNamesNoDeal", err)
	}
}

func TestAMalformedPayloadIsAnErrorRatherThanAnEmptyNote(t *testing.T) {
	_, _, err := roomNote(events.Envelope{
		Type:    dealrooms.EventCommentPosted,
		Entity:  events.EntityRef{Type: roomTimelineEntity, ID: ids.NewV7()},
		Payload: json.RawMessage(`{"deal_id": 12}`),
	})
	if err == nil {
		t.Fatal("a payload that does not decode produced no error, so the note would be written against no deal")
	}
}
