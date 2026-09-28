// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var testNow = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func openDeal() crmcontracts.Deal {
	return crmcontracts.Deal{
		Id: openapi_types.UUID(ids.NewV7()), Name: "Nordwind", Status: crmcontracts.DealStatusOpen,
	}
}

func act(kind crmcontracts.ActivityKind, at time.Time) crmcontracts.Activity {
	subject := "Something happened"
	return crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: kind, OccurredAt: at, Subject: &subject,
	}
}

func inboundMail(at time.Time) crmcontracts.Activity {
	a := act(crmcontracts.ActivityKindEmail, at)
	dir := crmcontracts.ActivityDirectionInbound
	a.Direction = &dir
	return a
}

// booked is what the deal's own next-meeting read answers. The rules take a
// meeting from there rather than from the timeline page, so a test that wants
// one in play seeds this and not a row in the list.
func booked(at time.Time) *activities.BookedMeeting {
	return &activities.BookedMeeting{ID: ids.NewV7(), Subject: "Detailabstimmung", StartsAt: at}
}

// seat is one stakeholder as the card reads them: named, and open to filed work,
// because most readers may do both and the tests about the exceptions say so.
func seat(role, name string) Seat {
	return Seat{Role: role, Name: name, ContactID: ids.NewV7(), Attachable: true}
}

func TestAnOpenTaskIsReusedInsteadOfDuplicated(t *testing.T) {
	taskID := ids.NewV7()
	f := facts{
		deal: openDeal(), now: testNow,
		timeline:  []crmcontracts.Activity{act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -12))},
		openTasks: []activities.OpenTask{{ID: taskID, Subject: "Agree the next step"}},
	}
	mv := decideMove(f)
	if mv.Action != ActionOpenTask || mv.Arguments == nil {
		t.Fatalf("existing work offered another write: %+v", mv)
	}
	if len(mv.Evidence) != 1 || mv.Evidence[0].ActivityId == nil || *mv.Evidence[0].ActivityId != openapi_types.UUID(taskID) {
		t.Fatalf("move does not name the existing task: %+v", mv)
	}
}

func TestABookedMeetingOutranksEverythingElse(t *testing.T) {
	f := facts{
		deal: openDeal(), now: testNow,
		nextMeeting: booked(testNow.AddDate(0, 0, 3)),
		timeline:    []crmcontracts.Activity{inboundMail(testNow.AddDate(0, 0, -3))},
	}
	if mv := decideMove(f); mv.Action != ActionOpenMeetingBrief {
		t.Fatalf("action = %q, want the meeting brief: a dated meeting beats an unanswered mail", mv.Action)
	}
}

func TestAnOutstandingRequestOffersAnEvidenceLinkedTask(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, requests: []crmcontracts.Activity{inboundMail(testNow.AddDate(0, 0, -3))}}
	mv := decideMove(f)
	if mv.Action != ActionCreateTask {
		t.Fatalf("action = %q, want a task accepting the request", mv.Action)
	}
	if mv.Arguments == nil || (*mv.Arguments)["request_activity_id"] == nil {
		t.Fatalf("the task names no source request: %+v", mv.Arguments)
	}
}

func TestTheMoveAndReplyToNameTheSameMail(t *testing.T) {
	for _, status := range []crmcontracts.DealStatus{crmcontracts.DealStatusOpen, crmcontracts.DealStatusWon, crmcontracts.DealStatusLost} {
		t.Run(string(status), func(t *testing.T) {
			request := inboundMail(testNow.AddDate(0, 0, -200))
			deal := openDeal()
			deal.Status = status
			f := facts{deal: deal, now: testNow, timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindNote, testNow)}, requests: []crmcontracts.Activity{request, inboundMail(testNow.AddDate(0, 0, -201))}}
			mv := decideMove(f)
			card := composeDeterministic(f, mv)
			if mv.Action != ActionCreateTask || mv.Arguments == nil || (*mv.Arguments)["request_activity_id"] != request.Id || card.ReplyTo == nil || *card.ReplyTo != request.Id {
				t.Fatalf("request was lost or its two actions disagree: move=%+v card=%+v", mv, card)
			}
			in := project(f, mv)
			if !citableIDs(in)[request.Id.String()] {
				t.Fatal("old request lost its evidence outside the timeline")
			}
			if _, ok := citedRecord(f, request.Id.String()); !ok {
				t.Fatal("old request cannot be cited")
			}
		})
	}
}

func TestLastContactIgnoresWhatIsOnlyScheduled(t *testing.T) {
	// The timeline is newest first and holds booked meetings at the top, which
	// are plans rather than contact. Counting one as contact prints "the last
	// contact was 3 days ago" about a meeting that has not happened.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{
			act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 20)),
			act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -6)),
		},
	}
	last, ok := lastContact(f)
	if !ok {
		t.Fatal("a deal with a past mail has had contact")
	}
	if last.Kind != crmcontracts.ActivityKindEmail {
		t.Fatalf("last contact = %v, want the mail: a meeting 20 days out has not happened", last.Kind)
	}
}

func TestACreateTaskArgumentIsAReadyTaskBody(t *testing.T) {
	// The click sends these arguments as the task's body, unedited. A missing
	// link or source is a task the server refuses after the reader clicked.
	mv := decideMove(facts{deal: openDeal(), now: testNow})
	if mv.Action != ActionCreateTask || mv.Arguments == nil {
		t.Fatalf("move = %+v, want a create_task carrying a body", mv)
	}
	args := *mv.Arguments
	if args["subject"] == "" || args["subject"] == nil {
		t.Fatalf("the task carries no subject: %v", args)
	}
	if args["source"] != "manual" {
		t.Fatalf("source = %v, want manual", args["source"])
	}
	links, ok := args["links"].([]map[string]any)
	if !ok || len(links) != 1 {
		t.Fatalf("links = %v, want the deal it belongs to", args["links"])
	}
	if links[0]["entity_type"] != "deal" {
		t.Fatalf("link = %v, want the deal", links[0])
	}
}

func TestAClosedDealGetsNoMove(t *testing.T) {
	deal := openDeal()
	deal.Status = crmcontracts.DealStatusWon
	if mv := decideMove(facts{deal: deal, now: testNow}); mv.Action != ActionNone {
		t.Fatalf("action = %q, want none: a won deal has no next step", mv.Action)
	}
}

func TestAWithheldRowIsNeverNamedAsTheOperand(t *testing.T) {
	// The verb this arm names gates on content, so pointing it at a row the
	// reader may not open would render a button that 404s.
	withheldMail := inboundMail(testNow.AddDate(0, 0, -3))
	state := crmcontracts.ActivityContentStateWithheld
	withheldMail.ContentState = &state
	withheldMail.Subject = nil
	f := facts{deal: openDeal(), now: testNow, requests: []crmcontracts.Activity{withheldMail}}
	if _, ok := unansweredInbound(f); ok {
		t.Fatal("a withheld mail was offered as the one to answer")
	}
}

// A deal nobody has contacted yet used to be told "Nothing has been logged on
// this deal yet — agree the next step", which restates the empty timeline the
// reader is already looking at. These tests pin the sentence that replaced it:
// who to write to, and the count a reader can check it against.

func TestTheFirstMoveOnAnUncontactedDealNamesWhoToOpenWith(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "blocker", Name: "Patrick Ganzmann"},
		{Role: "champion", Name: "Roland Martinez"},
		{Role: "economic_buyer", Name: "Philipp Königs"},
	}}

	mv := decideMove(f)

	if mv.Action != ActionDraftEmail {
		t.Errorf("the move on an uncontacted deal is %q, not a first email", mv.Action)
	}
	if !strings.Contains(mv.Reason, "Roland Martinez") {
		t.Errorf("the advice does not name who to open with: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the advice does not say why that contact: %q", mv.Reason)
	}
	// The count is what makes the sentence checkable against the page.
	if !strings.Contains(mv.Reason, "3 contacts") {
		t.Errorf("the advice does not say how many are named: %q", mv.Reason)
	}
}

func TestTheFirstMoveNeverOpensWithTheBlocker(t *testing.T) {
	// A deal whose ONLY named seat is the contact most likely to refuse it. No
	// advice is the right answer; naming them would be worse than silence.
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "blocker", Name: "Patrick Ganzmann"},
	}}

	mv := decideMove(f)

	if mv.Action == ActionDraftEmail {
		t.Error("the card told the rep to open the deal by writing to its blocker")
	}
	if strings.Contains(mv.Reason, "Patrick Ganzmann") {
		t.Errorf("the blocker was named as the way in: %q", mv.Reason)
	}
}

func TestTheFirstMoveTakesTheBestAvailableRole(t *testing.T) {
	// No champion: the economic buyer is the next best answer, not a fallback
	// to "agree the next step".
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "influencer", Name: "Ines Eschbacher"},
		{Role: "economic_buyer", Name: "Philipp Königs"},
	}}

	mv := decideMove(f)

	if !strings.Contains(mv.Reason, "Philipp Königs") {
		t.Errorf("the economic buyer was not chosen over the influencer: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "economic buyer") {
		t.Errorf("the role is not said in words a sentence can carry: %q", mv.Reason)
	}
}

func TestASeatTheReaderMayNotNameStillCarriesItsRole(t *testing.T) {
	// The reader holds deal:read without contact:read, so the seam supplies the
	// seats unnamed. The role is not the secret and the advice still works.
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{{Role: "champion"}}}

	mv := decideMove(f)

	if mv.Action != ActionDraftEmail {
		t.Errorf("an unnamed champion produced no opening move: %q", mv.Action)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the role was dropped along with the name: %q", mv.Reason)
	}
}

func TestAContactedDealKeepsItsOwnMove(t *testing.T) {
	// The opening move is for a deal with NO contact. Once somebody has
	// written, the later rules own the answer and this must not displace them.
	f := facts{
		deal:     openDeal(),
		now:      testNow,
		timeline: []crmcontracts.Activity{inboundMail(testNow.Add(-24 * time.Hour))},
		seats:    []Seat{{Role: "champion", Name: "Roland Martinez"}},
	}

	mv := decideMove(f)

	if strings.Contains(mv.Reason, "Nobody has been contacted") {
		t.Errorf("a deal with an inbound mail was called uncontacted: %q", mv.Reason)
	}
}

// The deal this whole change came from: contacted, quiet, nothing booked, and
// the only open task minted by the product. The old card said "complete the
// existing task"; the contact page said "book a meeting with the champion". The
// records said the contact page was right.
func TestAQuietContactedDealNamesTheChampionToMeet(t *testing.T) {
	champion := seat("champion", "Annabelle Malherbe")
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -23))},
		seats:    []Seat{{Role: "blocker", Name: "Nadine Pichelot"}, champion},
	}

	mv := decideMove(f)

	if mv.Action != ActionCreateTask {
		t.Fatalf("action = %q, want the meeting filed as work", mv.Action)
	}
	subject, _ := (*mv.Arguments)["subject"].(string)
	if subject != "Book a meeting with Annabelle Malherbe" {
		t.Errorf("the filed task does not say what it is for: %q", subject)
	}
	if !strings.Contains(mv.Reason, "23 days ago") {
		t.Errorf("the advice does not say how long it has been quiet: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "nothing is booked") {
		t.Errorf("the advice does not say why now: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the advice does not say why that contact: %q", mv.Reason)
	}
	// Both records: the deal it is about, and the contact it is with.
	links, _ := (*mv.Arguments)["links"].([]map[string]any)
	if len(links) != 2 {
		t.Fatalf("the task is not filed against both the deal and the contact: %+v", links)
	}
	if links[1]["entity_id"] != champion.ContactID {
		t.Errorf("the task names a contact nobody chose: %+v", links[1])
	}
}

func TestTheMeetingRequestTakesTheBestAvailableRole(t *testing.T) {
	// No champion on the deal. The economic buyer is the next best answer, and
	// the influencer beside them is not.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -30))},
		seats:    []Seat{seat("influencer", "Ines Eschbacher"), seat("economic_buyer", "Philipp Königs")},
	}

	mv := decideMove(f)

	if !strings.Contains(mv.Reason, "Philipp Königs") {
		t.Errorf("the economic buyer was not chosen over the influencer: %q", mv.Reason)
	}
}

func TestTheMeetingRequestNeverNamesTheBlocker(t *testing.T) {
	// The same refusal the opening move makes: the contact most likely to
	// refuse the deal is not who to send a rep to meet.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -30))},
		seats:    []Seat{seat("blocker", "Nadine Pichelot")},
	}

	mv := decideMove(f)

	if strings.Contains(mv.Reason, "Nadine Pichelot") {
		t.Errorf("the blocker was offered as the meeting to book: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "agree the next step") {
		t.Errorf("the card did not fall back to the sentence that names nobody: %q", mv.Reason)
	}
}

func TestASeatTheReaderMayNotFileAgainstIsNamedByRoleOnly(t *testing.T) {
	// A read-only share shows this reader the champion and does not let them add
	// to that contact's record. Two things follow, and the second is the one
	// that took a review to see.
	//
	// The link is withheld, because a task pointing at a contact the reader may
	// not write to fails at the server after the click.
	//
	// And the NAME goes with it. The card is stored and served again from the
	// queue, where the only id that survives is the one in the link — so a move
	// naming somebody it does not link is a name nothing can ever re-check, and
	// it would still be printed after the reader lost the contact entirely. The
	// role is what the card says instead, exactly as when it may not read the
	// name at all.
	champion := seat("champion", "Annabelle Malherbe")
	champion.Attachable = false
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -23))},
		seats:    []Seat{champion},
	}

	mv := decideMove(f)

	if strings.Contains(mv.Reason, "Annabelle Malherbe") {
		t.Errorf("a contact was named in a move that cannot carry their id: %q", mv.Reason)
	}
	if subject, _ := (*mv.Arguments)["subject"].(string); strings.Contains(subject, "Annabelle Malherbe") {
		t.Errorf("the filed task names a contact the move cannot link: %q", subject)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the advice dropped the role along with the name: %q", mv.Reason)
	}
	links, _ := (*mv.Arguments)["links"].([]map[string]any)
	if len(links) != 1 {
		t.Fatalf("a task was filed against a contact this reader may not write to: %+v", links)
	}
	if links[0][argEntityType] != linkDeal {
		t.Errorf("the surviving link is not the deal: %+v", links[0])
	}
}

func TestAFreshMoveNamesItsContactToTheCacheAndTheAudience(t *testing.T) {
	// NamedContacts reads the move BEFORE it has been through JSON, where the
	// links are Go maps and the ids are typed. It used to understand only the
	// stored shape and answered "this move names nobody" for every fresh one —
	// silently, which cost the fingerprint its operand and let the audience
	// check pass because it had found nothing to check.
	champion := seat("champion", "Annabelle Malherbe")
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -23))},
		seats:    []Seat{champion},
	}

	mv := decideMove(f)

	named := NamedContacts(mv)
	if len(named) != 1 || named[0] != champion.ContactID {
		t.Fatalf("NamedContacts = %v on a freshly built move, want the champion %s", named, champion.ContactID)
	}
	// And the same move once stored, which is the shape the queue re-reads.
	encoded, err := json.Marshal(mv)
	if err != nil {
		t.Fatalf("encoding the move: %v", err)
	}
	var stored crmcontracts.DealStatusCardMove
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatalf("decoding the stored move: %v", err)
	}
	if got := NamedContacts(stored); len(got) != 1 || got[0] != champion.ContactID {
		t.Fatalf("NamedContacts = %v on the stored move, want the same answer as the fresh one", got)
	}
}

func TestTheCacheKeyMovesWhenTheContactDoes(t *testing.T) {
	// Two stakeholders who share a display name. The sentence and the subject
	// are then byte-identical, and only the operand differs — so a key built
	// from the prose alone would serve the first contact's task body for the
	// second, filing work onto somebody nobody chose.
	base := func(contactID ids.UUID) facts {
		champion := seat("champion", "Alex Weber")
		champion.ContactID = contactID
		return facts{
			deal: openDeal(), now: testNow,
			timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -23))},
			seats:    []Seat{champion},
		}
	}
	first, second := base(ids.NewV7()), base(ids.NewV7())
	// Same deal, so nothing but the stakeholder differs.
	second.deal = first.deal

	firstMove, secondMove := decideMove(first), decideMove(second)
	if firstMove.Reason != secondMove.Reason {
		t.Fatalf("the fixture no longer tests what it claims: the two reasons differ\n%q\n%q",
			firstMove.Reason, secondMove.Reason)
	}
	if moveKey(firstMove) == moveKey(secondMove) {
		t.Fatal("two different stakeholders produced one cache key, so the stored card " +
			"would file the meeting onto whichever contact was written first")
	}
}

func TestAMeetingBookedBeyondTheHorizonOutranksOpeningOutreach(t *testing.T) {
	// Too far off to prepare for, and still an agreed next step. A deal nobody
	// has written to yet must not be told to open outreach as though nothing
	// had been arranged.
	f := facts{
		deal: openDeal(), now: testNow,
		nextMeeting: booked(testNow.AddDate(0, 0, 20)),
		seats:       []Seat{seat("champion", "Roland Martinez")},
	}

	mv := decideMove(f)

	if mv.Action != ActionOpenMeetingBrief {
		t.Fatalf("action = %q, want the booked meeting to be the answer", mv.Action)
	}
	if strings.Contains(mv.Reason, "Nobody has been contacted") {
		t.Errorf("a deal with a meeting on the calendar was told to open outreach: %q", mv.Reason)
	}
}

func TestAHousekeepingTaskDoesNotDecideTheMove(t *testing.T) {
	// The gather read excludes system-minted work in SQL, so by the time the
	// rules run there is nothing in openTasks to outrank the advice. This holds
	// the rules to that contract: a card whose only "work" was the product's
	// own reminder must still say what a colleague should do.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, -23))},
		seats:    []Seat{seat("champion", "Annabelle Malherbe")},
	}

	if mv := decideMove(f); mv.Action != ActionCreateTask ||
		!strings.Contains(mv.Reason, "Book a meeting") {
		t.Fatalf("a deal with no human task filed got no advice: %+v", mv)
	}
}

func TestADealWithNoSeatsSaysNothingItCannotKnow(t *testing.T) {
	// Nobody is named, so there is nobody to open with. The card falls back to
	// its old sentence rather than inventing a contact.
	f := facts{deal: openDeal(), now: testNow}

	mv := decideMove(f)

	if mv.Action == ActionDraftEmail {
		t.Error("the card advised writing an email on a deal with nobody to write to")
	}
}

// `arguments` is typed as an object in the contract, so a move that takes no
// operand ships an empty one.
//
// Asserted through the WIRE, not the Go value: a nil map behind a non-nil
// pointer is invisible in Go — `mv.Arguments != nil` passes — and only
// becomes `"arguments": null` when it is marshalled. A client that reads the
// field as an object gets null where it indexes.
func TestAMoveWithNoOperandShipsAnEmptyObjectNotNull(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "champion", Name: "Roland Martinez"},
	}}

	encoded, err := json.Marshal(decideMove(f))
	if err != nil {
		t.Fatalf("marshalling the move: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"arguments":null`)) {
		t.Errorf("the move serialized arguments as null, which the contract types as an object: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"arguments":{}`)) {
		t.Errorf("a move with no operand did not ship an empty object: %s", encoded)
	}
}

func TestRemindersAndNotesDoNotCountAsCustomerContact(t *testing.T) {
	request := inboundMail(testNow.AddDate(0, 0, -200))
	f := facts{deal: openDeal(), now: testNow, timeline: []crmcontracts.Activity{
		act(crmcontracts.ActivityKindTask, testNow), act(crmcontracts.ActivityKindNote, testNow), request,
	}}
	last, ok := lastContact(f)
	if !ok || last.Id != request.Id {
		t.Fatal("internal work reset the last customer contact")
	}
}

func TestACoveredRequestDoesNotOfferAnotherReadersPrivateTask(t *testing.T) {
	request := inboundMail(testNow.Add(-time.Hour))
	covered := true
	request.EmailSummary = &crmcontracts.EmailSummary{RequestHasReminder: &covered}
	f := facts{deal: openDeal(), now: testNow, requests: []crmcontracts.Activity{request}}
	mv := decideMove(f)
	if mv.Action != ActionDraftEmail {
		t.Fatalf("covered request offered %s", mv.Action)
	}
	if (*mv.Arguments)["activity_id"] != request.Id {
		t.Fatal("covered request must name only its readable source")
	}
	card := composeDeterministic(f, mv)
	if card.ReplyTo == nil || *card.ReplyTo != request.Id {
		t.Fatal("source reply remains unavailable")
	}
}
