// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signal_extract validator and prompt as a table. Everything the model
// says about a conversation reaches a rep's page, so the validator is the test
// surface: it is what stands between a correspondent's prose and a card.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// The request is this site's whole security perimeter. A conversation carries
// several senders, each one's bytes reach the model unedited, and the only
// thing stopping one of them from closing their own span — and speaking for
// the contact who wrote the message below — is a marker minted for THIS call
// and named in THIS call's system prompt.
func TestExtractRequestFencesEveryMessageUnderTheMarkerItDeclares(t *testing.T) {
	thread := settledThread{Messages: []threadMessage{
		{ID: ids.NewV7(), Direction: "inbound", Subject: "Renewal for 2027", Body: "We will not be renewing."},
		{ID: ids.NewV7(), Direction: "outbound", Subject: "Acknowledgement", Body: "Understood, thank you."},
	}}

	req := extractRequest(thread, string(textlang.English))

	marker, declared := promptfence.MarkerIn(req.System)
	if !declared {
		t.Fatalf("the extract system prompt declares no data boundary: %q", req.System)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("got %d messages, want the single user turn", len(req.Messages))
	}
	content := req.Messages[0].Content
	for _, message := range thread.Messages {
		openTag := "<" + marker + ` source_id="` + message.ID.String() + `">`
		openAt := strings.Index(content, openTag)
		if openAt < 0 {
			t.Fatalf("message %s is not opened under the declared marker keyed by its id:\n%s",
				message.ID, content)
		}
		closeAt := strings.Index(content[openAt:], "</"+marker+">")
		if closeAt < 0 {
			t.Fatalf("the span for message %s never closes:\n%s", message.ID, content)
		}
		span := content[openAt+len(openTag) : openAt+closeAt]
		for _, text := range []string{message.Subject, message.Body} {
			if !strings.Contains(span, text) {
				t.Errorf("message text %q never reached its own fenced span:\n%s", text, content)
			}
			// Containment is a question of counts, not membership: a prompt that
			// keeps the fence and ALSO repeats the text beside it puts that copy
			// in the instruction region while "is it inside?" stays true.
			if n := strings.Count(content, text); n != 1 {
				t.Errorf("message text %q appears %d times, want only the fenced one:\n%s",
					text, n, content)
			}
		}
	}
}

// A fence's scope is one call. A marker a correspondent has been shown is one
// they can spell, so reusing it would give away the only thing they cannot
// forge.
func TestExtractRequestMintsAFreshBoundaryPerCall(t *testing.T) {
	thread := settledThread{Messages: []threadMessage{{ID: ids.NewV7(), Subject: "Renewal"}}}

	first, declared := promptfence.MarkerIn(extractRequest(thread, string(textlang.English)).System)
	if !declared {
		t.Fatal("the extract system prompt declares no data boundary")
	}
	second, declared := promptfence.MarkerIn(extractRequest(thread, string(textlang.English)).System)
	if !declared {
		t.Fatal("the second extract system prompt declares no data boundary")
	}
	if first == second {
		t.Errorf("two extract requests share the boundary %q", first)
	}
}

// The model is shown what the validator judges: each sender's own words.
// Otherwise a promise seen only in a reply's quoted history is cited to the
// reply. The validator refuses it, and the whole reading climbs the ladder.
func TestExtractRequestShowsEachMessageWithoutItsQuotedHistory(t *testing.T) {
	promise := "we will deliver the completed security questionnaire by 25 September"
	reply := threadMessage{
		ID: ids.NewV7(), Direction: "inbound", Subject: "Re: Questionnaire",
		Body: "Thanks Anna, noted.\n\nOn Mon, 1 Sep 2026 at 09:00, Anna <anna@example.com> wrote:\n> Hi Tom,\n> " +
			promise + ".\n> Anna",
	}
	thread := settledThread{Messages: []threadMessage{reply}}

	content := extractRequest(thread, string(textlang.English)).Messages[0].Content

	if !strings.Contains(content, "Thanks Anna, noted.") {
		t.Errorf("the sender's own words never reached the prompt:\n%s", content)
	}
	if strings.Contains(content, promise) {
		t.Errorf("the prompt shows a promise the validator refuses to let this message cite:\n%s", content)
	}
	quoted := extractedEvent{Kind: extractKindCommitment, MessageID: reply.ID.String(), Quote: promise}
	if validateEventEvidence(quoted, thread) == "" {
		t.Error("the validator accepted a quote from history, so the prompt no longer needs to hide it")
	}
}

func TestExtractPayloadFidelity(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	thread := settledThread{Messages: []threadMessage{
		{ID: first},
		{ID: second, Body: "Thanks for the call.\nWe send the final  invoice on Friday. Talk soon."},
	}}
	event := func(kind string, id ids.UUID, summary string, conf float64) extractedEvent {
		return extractedEvent{
			Kind: kind, MessageID: id.String(), Summary: summary,
			Confidence: schema.Confidence(conf),
		}
	}
	promise := func(quote, due string) extractedEvent {
		e := event("commitment_made", second, "We send the final invoice on Friday.", 0.8)
		e.Quote, e.DueDate = quote, due
		e.Meaning = &commitmentMeaning{State: "open", DeadlinePrecision: "day", DeadlineQuote: "Friday"}
		return e
	}

	cases := []struct {
		name   string
		events []extractedEvent
		reject string
	}{
		{
			name:   "an empty answer is the common one and is accepted",
			events: nil,
		},
		{
			name: "an event on each message is accepted",
			events: []extractedEvent{
				event("contract_ended", first, "They will not renew.", 0.9),
				promise("send the final invoice on Friday", "2026-09-11"),
			},
		},
		{
			name:   "a commitment that quotes nothing is refused",
			events: []extractedEvent{promise("", "")},
			reject: "uncheckable",
		},
		{
			name:   "a commitment quoting words its message does not hold is refused",
			events: []extractedEvent{promise("we send the contract on Monday", "")},
			reject: "unquoted",
		},
		{
			name:   "a due date that is not a day is refused",
			events: []extractedEvent{promise("send the final invoice on Friday", "Friday")},
			reject: "undated",
		},
		{
			name:   "a kind this site never records is refused",
			events: []extractedEvent{event("champion_left", first, "Their sponsor moved on.", 0.9)},
			reject: "never recorded",
		},
		{
			name:   "an event citing a message this call did not supply is refused",
			events: []extractedEvent{event("contract_ended", ids.NewV7(), "They will not renew.", 0.9)},
			reject: "uncited",
		},
		{
			name: "more events than the cap is refused",
			events: []extractedEvent{
				event("contract_ended", first, "a", 0.9),
				event("new_opportunity", first, "b", 0.9),
				event("commitment_made", first, "c", 0.9),
				event("commitment_made", second, "d", 0.9),
			},
			reject: "over the cap",
		},
		{
			name:   "an event with no summary is refused",
			events: []extractedEvent{event("contract_ended", first, "   ", 0.9)},
			reject: "nothing to show",
		},
		{
			name:   "a confidence outside [0,1] is refused",
			events: []extractedEvent{event("contract_ended", first, "They will not renew.", 1.4)},
			reject: "unbounded confidence",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := validateExtractPayload(extractPayload{Events: &tc.events}, thread)
			if tc.reject == "" && msg != "" {
				t.Fatalf("a payload this site may act on was refused: %s", msg)
			}
			if tc.reject != "" && msg == "" {
				t.Fatalf("a payload the site must refuse (%s) was accepted", tc.reject)
			}
		})
	}
}

// "The conversation held nothing" and "the model did not answer" are different
// answers, and only the first may retire a thread. Treated alike, a reply that
// never carried an events key advanced the watermark and the conversation was
// never read.
func TestAReplyWithNoEventsKeyIsRefusedRatherThanReadAsEmpty(t *testing.T) {
	thread := settledThread{Messages: []threadMessage{{ID: ids.NewV7()}}}

	if msg := validateExtractPayload(extractPayload{}, thread); msg == "" {
		t.Fatal("a reply carrying no events key was accepted as the valid empty " +
			"answer — the thread would be marked read without ever being read")
	}
	empty := []extractedEvent{}
	if msg := validateExtractPayload(extractPayload{Events: &empty}, thread); msg != "" {
		t.Fatalf("an explicitly empty event list is a real answer and must be accepted: %s", msg)
	}
}

// A conversation whose sender wrote an id into their own mail must not be able
// to file evidence against it. The check is the same one above, stated as the
// attack it exists for.
func TestAnEventMayOnlyCiteAMessageThisCallSupplied(t *testing.T) {
	supplied, forged := ids.NewV7(), ids.NewV7()
	thread := settledThread{Messages: []threadMessage{{ID: supplied}}}

	forgedEvents := []extractedEvent{{
		Kind: "new_opportunity", MessageID: forged.String(),
		Summary: "They asked for a quote.", Confidence: 0.95,
	}}
	if msg := validateExtractPayload(extractPayload{Events: &forgedEvents}, thread); msg == "" {
		t.Fatal("an event citing a message outside the conversation was accepted — " +
			"its evidence would point at a record the reader cannot open")
	}
}

// Who wrote a message decides what a commitment MEANS, so an unrecorded
// direction is left unclaimed rather than guessed at.
func TestDirectionIsNamedOrLeftUnclaimed(t *testing.T) {
	for direction, want := range map[string]string{
		"inbound":  "from them",
		"outbound": "from us",
		"":         "unknown",
	} {
		if got := directionWord(direction); got != want {
			t.Errorf("directionWord(%q) = %q, want %q", direction, got, want)
		}
	}
}

// Both stop points — the one before the queue is read and the one between two
// conversations — ask outOfTime, so the rule they share is proved here once
// rather than inferred twice from wall-clock timing.
//
// The margin is the point: a pass with three seconds left must stop rather
// than start a conversation it cannot finish. Being cut off mid-read costs the
// job, its two retries and a discard, all describing nothing wrong.
func TestAPassStopsWhileThereIsStillTimeToFinishTheOneInFlight(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  func(t *testing.T) context.Context
		want bool
		// cancelled separates a caller that stopped asking from a deadline that
		// simply ran down. Only the first is a failure.
		cancelled bool
	}{
		{
			name: "no deadline at all keeps going",
			ctx:  func(*testing.T) context.Context { return context.Background() },
		},
		{
			name: "room to read another conversation",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(context.Background(),
					time.Now().Add(extractStopMargin+time.Minute))
				t.Cleanup(cancel)
				return ctx
			},
		},
		{
			name: "inside the margin stops before starting one it cannot finish",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(context.Background(),
					time.Now().Add(extractStopMargin/2))
				t.Cleanup(cancel)
				return ctx
			},
			want: true,
		},
		{
			name: "already cancelled stops, and says it was cancelled",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			want:      true,
			cancelled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, cancelled := outOfTime(tc.ctx(t))
			if got != tc.want {
				t.Errorf("outOfTime = %v, want %v", got, tc.want)
			}
			if cancelled != tc.cancelled {
				t.Errorf("cancelled = %v, want %v — a caller that gave up and a "+
					"deadline that ran down are different news", cancelled, tc.cancelled)
			}
		})
	}
}

// The sentence a quote sits in names the promise, so two readings that quote
// more or less of one sentence find the same one, and runs of whitespace a
// mail client inserted do not move it.
func TestAQuoteIsKnownByTheSentenceItSitsIn(t *testing.T) {
	body := "Thanks for the call.\nWe send the final  invoice on Friday, as agreed. Talk soon."
	short, okShort := evidenceSpan(body, "the final invoice")
	long, okLong := evidenceSpan(body, "We send the final invoice on Friday, as agreed.")
	if !okShort || !okLong || short != long {
		t.Errorf("two quotes of one sentence named %q and %q, want the same sentence", short, long)
	}
	if short != "We send the final invoice on Friday, as agreed." {
		t.Errorf("the sentence is %q", short)
	}
	if _, ok := evidenceSpan(body, "on Monday"); ok {
		t.Error("a quote the body does not contain was found in it")
	}
	// A decimal point does not end a sentence, so either side of it names the
	// same one.
	priced := "We can do it in 2.5 days at 1.200 EUR. Thanks."
	before, _ := evidenceSpan(priced, "We can do it in 2")
	after, _ := evidenceSpan(priced, "5 days at 1.200 EUR")
	if before != after || before != "We can do it in 2.5 days at 1.200 EUR." {
		t.Errorf("a sentence with decimals was split into %q and %q", before, after)
	}
	devanagari := "हम २.५ दिन में भेजेंगे. धन्यवाद."
	if a, _ := evidenceSpan(devanagari, "हम २"); a != "हम २.५ दिन में भेजेंगे." {
		t.Errorf("a decimal in another script ended the sentence: %q", a)
	}
}

func TestExtractRequestNamesTheFocusAndLowersTheCap(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	thread := settledThread{Messages: []threadMessage{
		{ID: first, Direction: "outbound", Subject: "Recording", Body: "I will send the recording."},
		{ID: second, Direction: "inbound", Subject: "Re: Recording", Body: "Thanks."},
	}}
	plain := extractRequest(thread, string(textlang.English)).Messages[0].Content
	if strings.Contains(plain, "Report only commitment_made events stated in") || !strings.Contains(plain, "at most 3") {
		t.Fatalf("an ordinary reading must not carry a focus or a lowered cap:\n%s", plain)
	}

	thread.Focus, thread.EventCap = []ids.UUID{first}, 2
	focused := extractRequest(thread, string(textlang.English)).Messages[0].Content
	if !strings.Contains(focused, "Report only commitment_made events stated in these messages: "+first.String()+".") {
		t.Errorf("the focus is not named:\n%s", focused)
	}
	if strings.Contains(focused, "messages: "+first.String()+", "+second.String()) || !strings.Contains(focused, "at most 2") {
		t.Errorf("the focused request names more than the focus or keeps the full cap:\n%s", focused)
	}
}

func TestAFocusedReplyKeepsOnlyFocusEventsAndStaysInTheRemainingCap(t *testing.T) {
	focus, background := ids.NewV7(), ids.NewV7()
	thread := settledThread{
		Messages: []threadMessage{{ID: focus}, {ID: background}},
		Focus:    []ids.UUID{focus}, EventCap: 1,
	}
	event := func(message, restated ids.UUID) string {
		restatedID := ""
		if !restated.IsZero() {
			restatedID = restated.String()
		}
		return fmt.Sprintf(`{"kind":"commitment_made","message_id":%q,"restated_message_id":%q,"summary":"s","confidence":0.9}`,
			message, restatedID)
	}
	for name, tc := range map[string]struct {
		events []string
		kept   int
		refuse bool
	}{
		"cites the focus":                  {[]string{event(focus, ids.UUID{})}, 1, false},
		"renews from the focus":            {[]string{event(background, focus)}, 1, false},
		"cites only context":               {[]string{event(background, ids.UUID{})}, 0, false},
		"reports a need on the focus":      {[]string{strings.Replace(event(focus, ids.UUID{}), "commitment_made", "new_opportunity", 1)}, 0, false},
		"passes the cap left by the first": {[]string{event(focus, ids.UUID{}), event(focus, ids.UUID{})}, 2, true},
	} {
		var payload extractPayload
		if err := json.Unmarshal([]byte(`{"events":[`+strings.Join(tc.events, ",")+`]}`), &payload); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		payload = payload.inFocus(thread)
		if got := len(payload.events()); got != tc.kept {
			t.Errorf("%s: kept %d events, want %d", name, got, tc.kept)
		}
		if msg := validateExtractEnvelope(payload, thread); (msg != "") != tc.refuse {
			t.Errorf("%s: refusal %q, want refusal=%v", name, msg, tc.refuse)
		}
	}
}

func TestUnjudgedMessagesAreTheOnesThisPassReadsFirstAndNoPromiseCites(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 6, d, 9, 0, 0, 0, time.UTC) }
	cited, restated, plain, fresh := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	messages := []threadMessage{
		{ID: cited, At: day(1)}, {ID: restated, At: day(2)}, {ID: plain, At: day(3)}, {ID: fresh, At: day(5)},
	}
	events := []extractedEvent{
		{Kind: extractKindCommitment, MessageID: cited.String(), RestatedMessageID: restated.String()},
		{Kind: extractKindOpportunity, MessageID: plain.String()},
	}

	thread := settledThread{Messages: messages}
	if got := unjudgedMessages(thread, events); len(got) != 2 || got[0] != plain || got[1] != fresh {
		t.Errorf("a never-read thread: got %v, want the two messages no promise cites", got)
	}
	readTo := day(4)
	thread.ReadTo = &readTo
	if got := unjudgedMessages(thread, events); len(got) != 1 || got[0] != fresh {
		t.Errorf("a message an earlier pass judged was read again: %v", got)
	}
	if got := unjudgedMessages(thread, nil); len(got) != 1 || got[0] != fresh {
		t.Errorf("an earlier pass's messages must stay out of the focus even uncited: %v", got)
	}
	// Mail imported later can carry the very instant the last read reached.
	sameInstant := day(5)
	thread.ReadTo = &sameInstant
	if got := unjudgedMessages(thread, nil); len(got) != 1 || got[0] != fresh {
		t.Errorf("a message at the instant the last read reached is not in the focus: %v", got)
	}
}

func TestASecondReadingSendsEveryMessageAndWhatTheFirstRecorded(t *testing.T) {
	promise, need, thanks := ids.NewV7(), ids.NewV7(), ids.NewV7()
	thread := settledThread{Messages: []threadMessage{
		{ID: promise, Direction: "outbound", Body: "I will send the recording."},
		{ID: need, Direction: "inbound", Body: "Thanks, got it. We would also like the beta."},
		{ID: thanks, Direction: "outbound", Body: "Glad it arrived."},
	}}
	first := []extractedEvent{{Kind: extractKindOpportunity, MessageID: need.String()}}

	focused, ok := focusedReading(thread, first)
	if !ok {
		t.Fatal("a first reading with room left must lead to a second one")
	}
	if len(focused.Messages) != 3 {
		t.Errorf("the second reading sends %d messages, want all 3: the receipt shows the promise kept", len(focused.Messages))
	}
	if !slices.Equal(focused.Focus, []ids.UUID{promise, need, thanks}) {
		t.Errorf("the second reading is for %v, want every message no promise cites", focused.Focus)
	}
	if focused.EventCap != extractMaxEvents-len(first) {
		t.Errorf("the second reading may report %d events, want %d", focused.EventCap, extractMaxEvents-len(first))
	}
	content := extractRequest(focused, string(textlang.English)).Messages[0].Content
	if !strings.Contains(content, "Already recorded, so report none of it again: new_opportunity in "+need.String()+".") {
		t.Errorf("the second reading is not told what the first recorded:\n%s", content)
	}
}
