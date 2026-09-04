// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for owed_verdict/owed.
//
// It certifies the shipped path rather than a description of it: the request
// comes from owedRequest, the builder the engine calls, and the reply is judged
// by validateOwedPayload, the validator the engine applies. A case that rebuilt
// either would measure a copy, and a copy stays green through the change that
// breaks the original.
//
// What the expectation MEANS: one verdict per fixture message, in fixture order.
// Position is the identity the corpus can name, because the ids are minted here
// and the fixture supplies no key of its own — and the obvious candidate, the
// subject, is not unique in a real backlog.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// owedFixture is ONE batch, as the backlog read hands it to the engine.
type owedFixture []owedFixtureMessage

// owedFixtureMessage is one backlog row in exactly the fields the prompt
// consumes. It carries nothing the corpus asserts and no id — see Prepare.
//
// The envelope fields are here because they are half the question this site
// answers: a report sent to a desk address with the reader merely copied reads
// exactly like a direct request without them, and a fixture that omitted them
// would certify a prompt strictly weaker than the one production sends.
type owedFixtureMessage struct {
	Subject         string   `json:"subject"`
	Body            string   `json:"body"`
	To              []string `json:"to,omitempty"`
	Cc              []string `json:"cc,omitempty"`
	HasCalendarPart bool     `json:"has_calendar_part,omitempty"`
}

// owedVerdictCases serves the one site that judges whether a waiting message
// asks its recipient side for anything.
type owedVerdictCases struct{}

func (owedVerdictCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskOwedVerdict,
		Variant: "owed",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope narrows the record to the ONE call this case makes, because
// judging a batch is not always one call.
//
// The engine re-asks every below-floor message solo on the next routing rung,
// and the verdict a row ends up wearing — or the decision to leave it unjudged —
// can come from that second answer. A run here measures the first, so claiming
// the whole invocation would overstate what the record covers.
func (owedVerdictCases) CertifiedScope() string { return aitasks.ScopeSingleCall }

// Prepare turns one batch and the verdicts the scenario expects into a runnable
// case, MINTING an activity id per message rather than reading it from either.
//
// Production takes those ids from the backlog rows, which no model has seen, so
// the only way an answer can carry one is to have read it out of this call's
// prompt. A fixture supplying them would hand them to whoever authored the
// expected reply, and a model echoing back ids it was given would then be
// indistinguishable from one that judged the right messages.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (owedVerdictCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var messages owedFixture
	if err := json.Unmarshal(fixture, &messages); err != nil {
		return nil, fmt.Errorf("owed_verdict/owed: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableOwedBatch(messages); err != nil {
		return nil, err
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("owed_verdict/owed: the expected answer is not a list of verdicts: %w", err)
	}
	if err := refuseUnreachableVerdicts(want, len(messages)); err != nil {
		return nil, err
	}
	batch := make([]owedCandidate, len(messages))
	for i, m := range messages {
		batch[i] = owedCandidate{
			// The generated contract's own word, not a literal: this is the kind
			// the backlog read returns for mail, and a second spelling here
			// would certify a candidate the store never produces.
			ID: ids.NewV7(), Kind: string(crmcontracts.ActivityKindEmail),
			Subject: m.Subject, Body: m.Body,
			To: m.To, Cc: m.Cc, HasCalendarPart: m.HasCalendarPart,
		}
	}
	return &owedVerdictCase{batch: batch, expected: want}, nil
}

// refuseUnreadableOwedBatch names a batch the backlog read could never have
// returned: it caps a call at owedBatchSize rows and truncates every body to
// owedBodyLimit, so a fixture beyond either bound certifies a prompt the product
// cannot send.
func refuseUnreadableOwedBatch(messages owedFixture) error {
	if len(messages) == 0 {
		return errors.New("owed_verdict/owed: the fixture supplies no message, so there is nothing to judge")
	}
	if len(messages) > owedBatchSize {
		return fmt.Errorf(
			"owed_verdict/owed: the fixture carries %d messages, but one call judges at most %d",
			len(messages), owedBatchSize)
	}
	for i, m := range messages {
		if n := utf8.RuneCountInString(m.Body); n > owedBodyLimit {
			return fmt.Errorf(
				"owed_verdict/owed: message %d carries a body of %d characters, but the backlog read truncates every body to %d",
				i+1, n, owedBodyLimit)
		}
	}
	return nil
}

// refuseUnreachableVerdicts names an expectation the validator can never
// satisfy. Each would measure nothing for as long as it stayed in the corpus,
// and naming it here costs a parse where finding it later costs a paid run.
func refuseUnreachableVerdicts(want []string, messages int) error {
	if len(want) != messages {
		return fmt.Errorf(
			"owed_verdict/owed: the scenario expects %d verdicts for %d messages, and this site answers one verdict per message",
			len(want), messages)
	}
	for i, verdict := range want {
		if !owedVerdicts[verdict] {
			return fmt.Errorf(
				"owed_verdict/owed: the scenario expects %q for message %d, which is not asks_us|informs_us",
				verdict, i+1)
		}
	}
	return nil
}

// owedVerdictCase is one batch ready to be judged, closed over the minted ids
// and the verdict the scenario expects for each message.
type owedVerdictCase struct {
	batch    []owedCandidate
	expected []string
}

// Run issues the one request this site sends, bare: production wraps the same
// request in a shape-retry where the brain supports one, and re-asks a
// below-floor message solo on the next rung. A case doing either would certify
// the answer a model gives after being told to try again.
func (c *owedVerdictCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := owedRequest(c.batch)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("owed_verdict/owed: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then validateOwedPayload against the batch that was asked about — and only
// then asks whether the verdicts are the ones the scenario expects.
//
// The order is the meaning: a reply that fails the validator has no verdicts to
// disagree with, and every way a batch reply can break the id contract is
// unusable rather than wrong.
//
// The confidence floor is deliberately not applied. It is the engine's decision
// about what to do with an answer it already believes, not a judgement on
// whether the answer is usable, and folding it in would report a hedged correct
// verdict as a broken reply.
func (c *owedVerdictCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload owedPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	if msg := validateOwedPayload(payload, c.batch); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	if disagreements := c.disagreements(payload); len(disagreements) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(disagreements, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// disagreements names EVERY message the reply judges otherwise than the scenario
// expects, in fixture order — not the first: a run that judged one right and two
// wrong is not the near miss one line would read as.
//
// Named by position, which is where the expectation was written. The minted id
// names nothing an author could look up, and the subject is captured text with
// no business in a record.
func (c *owedVerdictCase) disagreements(payload owedPayload) []string {
	judged := make(map[string]string, len(payload.Results))
	for _, r := range payload.Results {
		judged[r.ID] = r.Verdict
	}
	var out []string
	for i, m := range c.batch {
		if answered := judged[m.ID.String()]; answered != c.expected[i] {
			out = append(out, fmt.Sprintf(
				"message %d is judged %q where the scenario expects %q", i+1, answered, c.expected[i]))
		}
	}
	return out
}
