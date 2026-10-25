// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for signal_extract/thread_events.
//
// It certifies the shipped path: the request comes from extractRequest and the
// reply is judged by validateExtractPayload, the same builder and the same
// validator the engine uses. A case that rebuilt either would measure a copy,
// and a copy stays green through the change that breaks the original.
//
// What the expectation MEANS here: the set of events the conversation states,
// each named by the KIND and the POSITION of the message it is stated in.
// Position is the identity the corpus can name, because the ids are minted
// here and never supplied — production takes them from real activity rows no
// model has seen, so an answer can only carry one by having read this call's
// prompt, which is exactly what the citation check exists to prove.
//
// The empty expectation is the important one. Most conversations state no
// material event, and a site that invents one on a quiet thread is worse than
// a silent site — it puts a card in front of a rep for nothing.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// signalExtractFixture is ONE conversation, oldest first, as the thread read
// hands it to the engine.
type signalExtractFixture []signalExtractMessage

// signalExtractMessage is one message in exactly the fields the prompt
// consumes. It carries no id — see Prepare.
type signalExtractMessage struct {
	Participants []certCommitmentParty `json:"participants,omitempty"`
	Direction    string                `json:"direction"`
	Subject      string                `json:"subject"`
	Body         string                `json:"body"`
	// Sent is the day the message went, YYYY-MM-DD. A relative deadline is
	// resolved against it, so a scenario that dates a promise names its day.
	Sent string `json:"sent,omitempty"`
	// Reported is the kind of event the first reading of the pass cited on this
	// message. A fixture with one is the second reading, built as the extractor
	// builds it; a fixture with none is the ordinary reading.
	Reported string `json:"reported,omitempty"`
}

// certSentDefault dates a message whose scenario does not, so every prompt
// carries a real day the way production's does.
const certSentDefault = "2026-09-07"

// signalExtractExpectation identifies an event, its cited message and verified promiser.
type signalExtractExpectation struct {
	Recipients      []string           `json:"recipients,omitempty"`
	LifecycleScope  string             `json:"lifecycle_scope,omitempty"`
	Workstream      string             `json:"workstream,omitempty"`
	Buyer           string             `json:"buyer,omitempty"`
	CommercialRole  string             `json:"commercial_role,omitempty"`
	Meaning         *commitmentMeaning `json:"meaning,omitempty"`
	Optional        bool               `json:"optional,omitempty"`
	Kind            string             `json:"kind"`
	Message         int                `json:"message"`
	RestatedMessage int                `json:"restated_message,omitempty"`
	DueDate         *string            `json:"due_date,omitempty"`
	Promiser        string             `json:"promiser,omitempty"`
}

// signalExtractCases serves the one site that reads material events out of a
// settled conversation.
type signalExtractCases struct{}

func (signalExtractCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskSignalExtract,
		Variant: "thread_events",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope is the site's own kind: reading a conversation IS one call.
// There is no solo re-ask here — an event under the floor is dropped, not
// asked about again — so the call this case makes is the whole path.
func (signalExtractCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

// Prepare turns one conversation and the events the scenario expects into a
// runnable case, MINTING an activity id per message rather than reading one
// from either.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (signalExtractCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var messages signalExtractFixture
	if err := json.Unmarshal(fixture, &messages); err != nil {
		return nil, fmt.Errorf("signal_extract/thread_events: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableThread(messages); err != nil {
		return nil, err
	}
	var want []signalExtractExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"signal_extract/thread_events: the expected answer is not a list of events: %w", err)
	}
	if err := refuseUnreachableEvents(want, len(messages)); err != nil {
		return nil, err
	}
	thread := settledThread{Key: "cert-thread", CompanyID: ids.NewV7()}
	identities := map[string]ids.UUID{}
	references := make([]map[string]string, len(messages))
	var firstReading []extractedEvent
	for i, message := range messages {
		parties, refs, err := prepareCommitmentParties(message.Participants, identities)
		if err != nil {
			return nil, err
		}
		references[i] = refs
		sent := message.Sent
		if sent == "" {
			sent = certSentDefault
		}
		at, err := time.Parse(time.DateOnly, sent)
		if err != nil {
			return nil, fmt.Errorf("signal_extract/thread_events: message %d is sent on %q, which is not a day: %w",
				i+1, sent, err)
		}
		id := ids.NewV7()
		if message.Reported != "" {
			firstReading = append(firstReading, extractedEvent{Kind: message.Reported, MessageID: id.String()})
		}
		thread.Messages = append(thread.Messages, threadMessage{
			ID:        id,
			Parties:   parties,
			Direction: message.Direction,
			Subject:   message.Subject,
			Body:      message.Body,
			At:        at,
		})
	}
	numbered := thread.Messages
	if len(firstReading) > 0 {
		focused, ok := focusedReading(thread, firstReading)
		if !ok {
			return nil, errors.New(
				"signal_extract/thread_events: the fixture's first reading leaves nothing for a second one to read")
		}
		thread = focused
	}
	for i := range want {
		due := ""
		if want[i].DueDate != nil {
			due = *want[i].DueDate
		}
		if err := validateExpectedMeaning(want[i].Optional, want[i].Meaning, due, numbered[want[i].Message-1].Body); err != nil {
			return nil, err
		}
		if err := validateExpectedRecipients(want[i].Recipients, references[want[i].Message-1]); err != nil {
			return nil, err
		}
		for _, key := range []string{want[i].Promiser, want[i].Buyer} {
			if key == "" {
				continue
			}
			if _, found := references[want[i].Message-1][key]; !found {
				return nil, fmt.Errorf("expected participant %q is not a participant of message %d", key, want[i].Message)
			}
		}
	}
	return &signalExtractCase{
		thread: thread, numbered: numbered, expected: want, promiserKeys: certPromiserKeys(references...),
	}, nil
}

// refuseUnreadableThread names a conversation the thread read could never have
// returned. History reads add a recent window; every body still has the same size bound.
func refuseUnreadableThread(messages signalExtractFixture) error {
	if len(messages) == 0 {
		return errors.New(
			"signal_extract/thread_events: the fixture supplies no message, so there is no conversation to read")
	}
	if len(messages) > extractContextMessages {
		return fmt.Errorf(
			"signal_extract/thread_events: the fixture carries %d messages, but one call reads at most %d",
			len(messages), extractContextMessages)
	}
	for i, message := range messages {
		if n := utf8.RuneCountInString(message.Body); n > extractBodyLimit {
			return fmt.Errorf(
				"signal_extract/thread_events: message %d carries a body of %d characters, but the read truncates every body to %d",
				i+1, n, extractBodyLimit)
		}
		if message.Direction != "inbound" && message.Direction != "outbound" && message.Direction != "" {
			return fmt.Errorf(
				"signal_extract/thread_events: message %d is directed %q, and a captured email is inbound or outbound",
				i+1, message.Direction)
		}
	}
	return nil
}

// refuseUnreachableEvents names an expectation the validator can never satisfy,
// which would measure nothing for as long as it stayed in the corpus. An EMPTY
// expectation is not one of them — it is the abstention scenario, and it is the
// one most conversations deserve.
func refuseUnreachableEvents(want []signalExtractExpectation, messages int) error {
	if len(want) > extractMaxEvents {
		return fmt.Errorf(
			"signal_extract/thread_events: the scenario expects %d events, but this site reports at most %d",
			len(want), extractMaxEvents)
	}
	for i, event := range want {
		if _, ok := extractKinds[event.Kind]; !ok {
			return fmt.Errorf(
				"signal_extract/thread_events: expectation %d names kind %q, which this site never records",
				i+1, event.Kind)
		}
		if event.RestatedMessage != 0 && (event.RestatedMessage <= event.Message || event.RestatedMessage > messages) {
			return fmt.Errorf("signal_extract/thread_events: expectation %d has an invalid restatement position", i+1)
		}
		if event.Message < 1 || event.Message > messages {
			return fmt.Errorf(
				"signal_extract/thread_events: expectation %d cites message %d, and the conversation has %d",
				i+1, event.Message, messages)
		}
	}
	return nil
}

// signalExtractCase is one conversation ready to be read, closed over the
// minted ids and the events the scenario expects.
type signalExtractCase struct {
	promiserKeys map[string]string
	thread       settledThread
	// numbered is the fixture's conversation, which the expected events count
	// in. A second reading sends fewer messages than it holds.
	numbered []threadMessage
	expected []signalExtractExpectation
}

// Run issues the requests one pass sends, bare. Production wraps each in the
// shape-retry when the brain supports one. A case that did too would certify
// the answer a model gives after being told to try again.
func (c *signalExtractCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	// English, pinned, rather than the installation's base language: a
	// certification record grades a fixed corpus, and a score that moved with a
	// settings row would not be comparable between two installations or across
	// one that changed its mind. The rule is PRESENT in the graded request for
	// the same reason — production sends one, so a case that left it out would
	// grade a prompt the product does not send.
	req := extractRequest(c.thread, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("signal_extract/thread_events: %w", err)
	}
	trace.Output = resp.Text
	if len(c.thread.Focus) > 0 {
		// The fixture is already a second reading, and a pass makes no third.
		return trace, nil
	}
	return c.secondReading(ctx, completer, trace)
}

// secondReading finishes the pass the way readThread does, so the graded
// answer is the one the product files. A first reply the engine refuses is
// graded as it is. A refused second reply leaves the first reading's events,
// as keepFirstReading does.
func (c *signalExtractCase) secondReading(
	ctx context.Context, completer aitasks.Completer, trace aitasks.Trace,
) (aitasks.Trace, error) {
	var first extractPayload
	if json.Unmarshal([]byte(ai.Unfence(trace.Output)), &first) != nil || validateExtractEnvelope(first, c.thread) != "" {
		return trace, nil
	}
	focused, ok := focusedReading(c.thread, first.events())
	if !ok {
		return trace, nil
	}
	req := extractRequest(focused, string(textlang.English))
	trace.Requests = append(trace.Requests, req)
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("signal_extract/thread_events: the second reading: %w", err)
	}
	var second extractPayload
	if json.Unmarshal([]byte(ai.Unfence(resp.Text)), &second) != nil || validateExtractEnvelope(second, focused) != "" {
		return trace, nil
	}
	both := append(first.events(), second.events()...)
	served, err := json.Marshal(extractPayload{Events: &both})
	if err != nil {
		return trace, fmt.Errorf("signal_extract/thread_events: the two readings cannot be joined: %w", err)
	}
	trace.Output = string(served)
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then validateExtractPayload against the conversation that was asked about —
// and only then asks whether the events are the ones the scenario expects. A
// reply that fails the validator has no events to disagree with.
//
// The confidence floor is deliberately not applied. It is the engine's
// decision about what to do with an answer it already believes, not a
// judgement on whether the answer is usable, and folding it in would report a
// hedged correct reading as a broken reply.
func (c *signalExtractCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload extractPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	if msg := validateExtractPayload(payload, c.thread); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	if disagreements := c.disagreements(payload); len(disagreements) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(disagreements, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// AnswerConfidence is the confidence range over the commitments the reply
// reports: the one reading the task-creation threshold is set against.
func (c *signalExtractCase) AnswerConfidence(trace aitasks.Trace) (low, high float64, ok bool) {
	var payload extractPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return 0, 0, false
	}
	var confidences []float64
	for _, event := range payload.events() {
		if event.Kind == extractKindCommitment && event.Meaning.withheldOutcome() == "" {
			confidences = append(confidences, float64(event.Confidence))
		}
	}
	return spanOf(confidences)
}

// spanOf is the least and greatest of a set of confidences, and false for none.
func spanOf(confidences []float64) (low, high float64, ok bool) {
	if len(confidences) == 0 {
		return 0, 0, false
	}
	return slices.Min(confidences), slices.Max(confidences), true
}

// disagreements names every event the scenario expects and the reply missed,
// and every event the reply reports and the scenario does not — all of them,
// because a reading that found one of three is not the near miss one line
// would read as.
//
// Events are named by kind and by the message's position in the fixture, which
// is where the expectation was written. The minted id names nothing a scenario
// author could look up, and the mail's own text has no business being echoed
// into a record.
func (c *signalExtractCase) disagreements(payload extractPayload) []string {
	position := make(map[string]int, len(c.numbered))
	for i, message := range c.numbered {
		position[message.ID.String()] = i + 1
	}
	reported := map[string]bool{}
	for _, event := range payload.events() {
		if event.Kind == extractKindCommitment && event.Meaning.withheldOutcome() != "" {
			continue
		}
		reported[fmt.Sprintf("%s@%d:%s", event.Kind, position[event.MessageID], c.promiserKeys[event.PromiserRef])] = true
	}
	wanted := map[string]bool{}
	for _, event := range c.expected {
		if event.Optional {
			continue
		}
		wanted[fmt.Sprintf("%s@%d:%s", event.Kind, event.Message, event.Promiser)] = true
	}
	var out []string
	for _, expected := range c.expected {
		for _, event := range payload.events() {
			if event.Kind != expected.Kind || position[event.MessageID] != expected.Message {
				continue
			}
			if c.promiserKeys[event.PromiserRef] == expected.Promiser && !expectedRecipientsMatch(expected.Recipients, event.RecipientRefs, c.promiserKeys) {
				out = append(out, "the recipients do not match the quoted promise")
			}
			if expected.LifecycleScope != "" && event.LifecycleScope != expected.LifecycleScope {
				out = append(out, "the lifecycle scope does not match the source")
			}
			if expected.Workstream != "" && event.Workstream != expected.Workstream {
				out = append(out, "the named workstream does not match the source")
			}
			if expected.Buyer != "" && c.promiserKeys[event.BuyerRef] != expected.Buyer {
				out = append(out, "the buyer does not match the verified participant")
			}
			if expected.CommercialRole != "" && event.CommercialRole != expected.CommercialRole {
				out = append(out, "the commercial role does not match the request")
			}
			if !expectedMeaningMatches(expected.Meaning, event.Meaning) {
				out = append(out, "the commitment meaning does not match the source")
			}
			if expected.RestatedMessage != 0 && position[event.RestatedMessageID] != expected.RestatedMessage {
				out = append(out, "the renewed promise does not cite its restatement")
			}
			if expected.DueDate != nil && event.DueDate != *expected.DueDate {
				out = append(out, "the deadline does not match its source date")
			}
		}
	}
	for key := range wanted {
		if !reported[key] {
			out = append(out, fmt.Sprintf("the conversation states %s and the reply does not report it", key))
		}
	}
	for key := range reported {
		if !wanted[key] {
			out = append(out, fmt.Sprintf("the reply reports %s, which the conversation does not state", key))
		}
	}
	// Map iteration is unordered, and a detail line that reshuffles between
	// runs reads as a different failure each time.
	sort.Strings(out)
	return out
}
