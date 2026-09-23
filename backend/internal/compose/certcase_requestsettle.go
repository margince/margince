// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for request_settlement/request_settle.
//
// It certifies the shipped path rather than a description of it: the request
// comes from settleRequest, the builder the engine calls, and the reply is
// judged by validateSettlePayload, the validator the engine applies.
//
// What the expectation MEANS: one verdict per fixture conversation, in fixture
// order. Position is the identity the corpus can name, because the ids are
// minted here and the fixture supplies none — a fixture carrying ids would hand
// them to whoever authored the expected reply, and a model echoing back ids it
// was given is indistinguishable from one that read the right conversations.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// settleFixture is ONE batch: several conversations, each judged on its own.
type settleFixture []settleFixtureThread

// settleFixtureThread is one conversation in exactly the fields the prompt
// consumes. The messages are oldest first, as the window hands them over, and
// the first must be the inbound request the settlement is about.
type settleFixtureThread struct {
	Messages []settleFixtureMessage `json:"messages"`
}

type settleFixtureMessage struct {
	// Direction is the corpus author's own vocabulary — inbound or outbound —
	// which the fold turns into the "from them" / "from us" the prompt reads,
	// through the engine's own directionWord rather than a second mapping that
	// could disagree with it.
	Direction string `json:"direction"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// settleExpectation is what one conversation's verdict must be, and where the
// scenario cares, what must be named as still owed.
type settleExpectation struct {
	Verdict string `json:"verdict"`
	// RemainingNames is a phrase the still_owed answer has to contain, lowered
	// before comparison.
	//
	// Several renderings may be separated by "|", and any one satisfies it: the
	// thing owed has more than one honest name, and a single token pinned a
	// language with it — this phrase is our own note to ourselves, and the
	// models write it in the thread's language as readily as the installation's.
	//
	// Empty asserts nothing about the wording — the verdict
	// alone is the claim — which is what a settled or unsure case wants.
	RemainingNames string `json:"remaining_names,omitempty"`
}

// requestSettleCases serves the one site that judges what our reply did.
type requestSettleCases struct{}

func (requestSettleCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskRequestSettlement,
		Variant: "request_settle",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope narrows the record to the ONE call this case makes: the engine
// re-asks every below-floor conversation solo on the next rung, and the verdict
// a row ends up wearing can come from that second answer.
func (requestSettleCases) CertifiedScope() string { return aitasks.ScopeSingleCall }

// Prepare turns one batch and the verdicts the scenario expects into a runnable
// case, MINTING a request id per conversation.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (requestSettleCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var threads settleFixture
	if err := json.Unmarshal(fixture, &threads); err != nil {
		return nil, fmt.Errorf("request_settlement/request_settle: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableSettleBatch(threads); err != nil {
		return nil, err
	}
	var want []settleExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("request_settlement/request_settle: the expected answer is not a list of verdicts: %w", err)
	}
	if err := refuseUnreachableSettlements(want, threads); err != nil {
		return nil, err
	}
	batch := make([]settleCandidate, len(threads))
	for i, thread := range threads {
		requestID := ids.NewV7()
		messages := make([]threadMessage, len(thread.Messages))
		for j, m := range thread.Messages {
			messages[j] = threadMessage{
				ID: ids.NewV7(), Direction: m.Direction,
				Subject: m.Subject, Body: m.Body,
			}
		}
		batch[i] = settleCandidate{
			Request:  activities.RepliedRequest{RequestID: requestID, NewestOutboundID: ids.NewV7()},
			Messages: messages,
		}
	}
	return &requestSettleCase{batch: batch, expected: want}, nil
}

// refuseUnreadableSettleBatch names a batch the candidate read could never have
// returned.
//
// The two-message floor is the load-bearing one: this site judges what OUR
// REPLY did, so a conversation with nothing but the request in it has no answer
// to judge and would certify the model's willingness to guess.
func refuseUnreadableSettleBatch(threads settleFixture) error {
	if len(threads) == 0 {
		return errors.New("request_settlement/request_settle: the fixture supplies no conversation, so there is nothing to judge")
	}
	if len(threads) > settleBatchSize {
		return fmt.Errorf(
			"request_settlement/request_settle: the fixture carries %d conversations, but one call judges at most %d",
			len(threads), settleBatchSize)
	}
	for i, thread := range threads {
		if len(thread.Messages) < 2 {
			return fmt.Errorf(
				"request_settlement/request_settle: conversation %d carries %d message(s); this site judges what our REPLY did, so a thread with no reply in it has nothing to judge",
				i+1, len(thread.Messages))
		}
		if thread.Messages[0].Direction != fixtureInbound {
			return fmt.Errorf(
				"request_settlement/request_settle: conversation %d opens with a %q message, but the request a settlement is about is always inbound",
				i+1, thread.Messages[0].Direction)
		}
		if !hasOutbound(thread) {
			return fmt.Errorf(
				"request_settlement/request_settle: conversation %d carries no outbound message, and the candidate read only ever offers a thread the workspace has answered",
				i+1)
		}
		for j, m := range thread.Messages {
			if n := utf8.RuneCountInString(m.Body); n > extractBodyLimit {
				return fmt.Errorf(
					"request_settlement/request_settle: conversation %d message %d carries a body of %d characters, but the window truncates every body to %d",
					i+1, j+1, n, extractBodyLimit)
			}
		}
		if len(thread.Messages) > extractThreadMessages {
			return fmt.Errorf(
				"request_settlement/request_settle: conversation %d carries %d messages, but one window reads at most %d",
				i+1, len(thread.Messages), extractThreadMessages)
		}
	}
	return nil
}

func hasOutbound(thread settleFixtureThread) bool {
	for _, m := range thread.Messages {
		if m.Direction == fixtureOutbound {
			return true
		}
	}
	return false
}

// refuseUnreachableSettlements names an expectation the validator can never
// satisfy, at parse time rather than after a paid run.
func refuseUnreachableSettlements(want []settleExpectation, threads settleFixture) error {
	if len(want) != len(threads) {
		return fmt.Errorf(
			"request_settlement/request_settle: the scenario expects %d verdicts for %d conversations, and this site answers one verdict per conversation",
			len(want), len(threads))
	}
	for i, expectation := range want {
		if !settleVerdicts[expectation.Verdict] {
			return fmt.Errorf(
				"request_settlement/request_settle: the scenario expects %q for conversation %d, which is not settled|still_owed|unsure",
				expectation.Verdict, i+1)
		}
		// A phrase asserted against a verdict that carries no prose is a claim
		// nothing can satisfy: the validator refuses `remaining` on anything but
		// still_owed, so the case would fail as invalid rather than wrong and
		// the scenario would measure the refusal instead of the judgement.
		if expectation.RemainingNames != "" && expectation.Verdict != activities.RequestStillOwed {
			return fmt.Errorf(
				"request_settlement/request_settle: conversation %d expects the answer to name %q, but its verdict is %q and only still_owed carries a remaining phrase",
				i+1, expectation.RemainingNames, expectation.Verdict)
		}
	}
	return nil
}

// requestSettleCase is one batch ready to be judged, closed over the minted ids
// and what the scenario expects for each conversation.
type requestSettleCase struct {
	batch    []settleCandidate
	expected []settleExpectation
}

// Run issues the one request this site sends, bare: production wraps the same
// request in a shape-retry where the brain supports one, and re-asks a
// below-floor conversation solo on the next rung.
func (c *requestSettleCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := settleRequest(c.batch)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("request_settlement/request_settle: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then validateSettlePayload against the batch that was asked about — and only
// then asks whether the verdicts are the ones the scenario expects.
//
// The confidence floor is deliberately not applied, for the reason the owed
// case gives: it is the engine's decision about what to do with an answer it
// already believes, and folding it in would report a hedged correct verdict as
// a broken reply.
func (c *requestSettleCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload settlePayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	if msg := validateSettlePayload(payload, c.batch); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	if faults := c.faults(payload); len(faults) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(faults, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// faults names every way this reply disagrees with the scenario.
//
// Keyed by id rather than by position: the validator has already proved the
// reply answers each requested id exactly once, so the answer for conversation
// N is the one carrying N's id — reading by position would report the right
// verdict against the wrong conversation the moment a model reordered its list.
func (c *requestSettleCase) faults(payload settlePayload) []string {
	answers := map[string]settleResult{}
	for _, r := range payload.Results {
		answers[r.ID] = r
	}
	var faults []string
	for i, expectation := range c.expected {
		answer := answers[c.batch[i].Request.RequestID.String()]
		if answer.Verdict != expectation.Verdict {
			faults = append(faults, fmt.Sprintf(
				"conversation %d is judged %q where the scenario expects %q",
				i+1, answer.Verdict, expectation.Verdict))
			continue
		}
		if expectation.RemainingNames == "" {
			continue
		}
		if !remainingNamesIt(answer.Remaining, expectation.RemainingNames) {
			faults = append(faults, fmt.Sprintf(
				"conversation %d is owed %q, which does not name %q",
				i+1, answer.Remaining, expectation.RemainingNames))
		}
	}
	return faults
}

// remainingNamesIt reports whether the model's remaining phrase names the thing
// the scenario says is owed, under any of the renderings it accepts.
//
// Held by: TestRemainingMayBeNamedInSeveralRenderings (internal/compose/certcase_requestsettle_test.go)
func remainingNamesIt(remaining, accepted string) bool {
	lowered := strings.ToLower(remaining)
	for _, rendering := range strings.Split(accepted, "|") {
		rendering = strings.TrimSpace(rendering)
		if rendering != "" && strings.Contains(lowered, strings.ToLower(rendering)) {
			return true
		}
	}
	return false
}
