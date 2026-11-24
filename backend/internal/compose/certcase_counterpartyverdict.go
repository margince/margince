// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for capture_counterparty_verdict/verdict.
//
// It certifies the shipped path rather than a description of it. It drives the
// engine's own judgment of one sender. The request comes from verdictRequest and
// the reply is read by the engine's validator. The confidence floors and the one
// re-ask run as they ship. A case that rebuilt any of them would measure a copy,
// and a copy stays green through the change that breaks the original.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// counterpartyVerdictFixture is one first-time sender, in exactly the fields
// the ledger row hands the engine. It carries nothing the corpus asserts and no
// id — see Prepare for both.
type counterpartyVerdictFixture struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	// Direction and WroteBack carry the half of the judgment that is not in the
	// text. A scenario that omits them describes a sender whose direction was
	// never recorded, which is a real ledger row and reads as it did before.
	Direction string `json:"direction"`
	WroteBack bool   `json:"wrote_back"`
}

// counterpartyVerdictCases serves the one site that judges a first-time sender.
type counterpartyVerdictCases struct{}

func (counterpartyVerdictCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskCaptureCounterpartyVerdict,
		Variant: "verdict",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare turns one sender and the verdict the scenario expects into a runnable
// case, MINTING the ledger row id rather than reading it from either.
//
// Production takes that id from the ledger row, which no model has ever seen, so
// the only way an answer can carry it is to have read it out of this call's
// prompt. A fixture that supplied the id would put it in the hands of whoever
// authored the expected reply, and a model echoing back an id it was handed
// would then be indistinguishable from one that answered about the right sender
// — which is exactly the confusion validateVerdictPayload exists to prevent.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (counterpartyVerdictCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f counterpartyVerdictFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("capture_counterparty_verdict/verdict: the fixture is not the shape this site takes: %w", err)
	}
	// A correct reply differs from an incorrect one in the verdict token alone,
	// so the expectation IS that token rather than a wrapper carrying it.
	var want string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"capture_counterparty_verdict/verdict: the expected answer is not a verdict token: %w", err)
	}
	// An expectation outside the closed vocabulary is unreachable: the validator
	// refuses every reply that could satisfy it, so the scenario would measure
	// nothing for as long as it stayed in the corpus. Naming it here costs a
	// parse; finding it later costs a paid run.
	if _, known := statusForKind(want); !known {
		return nil, fmt.Errorf(
			"capture_counterparty_verdict/verdict: the scenario expects %q, which is not a sender kind", want)
	}
	row := capture.PendingCounterparty{
		ID:          ids.NewV7(),
		DisplayName: f.DisplayName,
		Email:       f.Email,
		Domain:      domainOf(f.Email),
		Subject:     f.Subject,
		Body:        f.Body,
		Direction:   f.Direction,
		WroteBack:   f.WroteBack,
	}
	// The engine settles some senders from the address alone and asks no model,
	// so a run over one would make no call and certify nothing.
	if certVerdictEngine(nil, nil).settledByTheAddress(row) {
		return nil, fmt.Errorf(
			"capture_counterparty_verdict/verdict: the engine settles %q from its address and asks no model, so this scenario measures nothing", f.Email)
	}
	return &counterpartyVerdictCase{row: row, expected: want}, nil
}

// counterpartyVerdictCase is one fixture sender ready to be judged, closed over
// the minted row id and the verdict the scenario expects.
type counterpartyVerdictCase struct {
	row      capture.PendingCounterparty
	expected string
}

// Run judges the sender through the engine's own path: the request, the
// validator, the floors, the one re-ask and the retirement to a human. The case
// records what the ledger is told. The calls go bare, through verdictRecorder.
func (c *counterpartyVerdictCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	brain := &verdictRecorder{completer: completer}
	ledger := &verdictRunLedger{}
	_, err := certVerdictEngine(brain, ledger).judgeOne(ctx, c.row, reAskBudgetFor(capture.DefaultVerdictPassCap))
	trace := aitasks.Trace{Requests: brain.requests}
	run := verdictRun{Replies: brain.replies, Settled: ledger.settled, Confidence: ledger.measured.Confidence, Retired: ledger.retired}
	var unusable *unusableVerdictAnswer
	switch {
	case brain.failed != nil:
		return trace, fmt.Errorf("capture_counterparty_verdict/verdict: %w", brain.failed)
	case err == nil:
	case errors.As(err, &unusable):
		run.Unusable = unusable.Error()
	default:
		run.Refused = err.Error()
	}
	output, err := json.Marshal(run)
	if err != nil {
		return trace, fmt.Errorf("capture_counterparty_verdict/verdict: recording the run: %w", err)
	}
	trace.Output = string(output)
	return trace, nil
}

// certVerdictEngine is the engine over the parts a run needs. It reads no
// allowlist, as a deployment that declared none, and stands the ledger in place
// of the database.
func certVerdictEngine(brain completer, ledger verdictLedger) *CounterpartyVerdictEngine {
	return &CounterpartyVerdictEngine{
		brain: brain, ledger: ledger, transactional: capture.NewTransactionalList(nil, nil),
		log: slog.New(slog.DiscardHandler),
	}
}

// domainOf is the part of an address after its last "@", as the ledger stores it.
func domainOf(email string) string {
	_, domain, _ := strings.Cut(email, "@")
	return strings.ToLower(domain)
}

// verdictRun is what a run records: the replies the model gave, and what the
// engine did with them. That is the kind it applied and how sure the model was, or
// the reason it retired the row to a human. A reply the engine could not read, or
// an answer it refused to act on, is kept apart.
type verdictRun struct {
	Replies    []string `json:"replies"`
	Settled    string   `json:"settled,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Retired    string   `json:"retired,omitempty"`
	Unusable   string   `json:"unusable,omitempty"`
	Refused    string   `json:"refused,omitempty"`
}

// verdictRunLedger is the ledger of one run: it decides nothing and records what
// the engine settled. No owner has spoken and no earlier answer exists, as for a
// first-time sender.
type verdictRunLedger struct {
	settled  string
	measured capture.VerdictMeasurement
	retired  string
}

func (*verdictRunLedger) OwnerDecided(context.Context, capture.PendingCounterparty) (bool, string, error) {
	return false, "", nil
}

func (*verdictRunLedger) TimesJudgedNotAContact(context.Context, string) (int, error) { return 0, nil }

func (l *verdictRunLedger) Settle(
	_ context.Context, _ capture.PendingCounterparty, kind string, _ bool, measured capture.VerdictMeasurement,
) (bool, error) {
	l.settled, l.measured = kind, measured
	return true, nil
}

func (l *verdictRunLedger) Retire(
	_ context.Context, _ capture.PendingCounterparty, reason string, measured capture.VerdictMeasurement,
) error {
	l.retired, l.measured = reason, measured
	return nil
}

// verdictRecorder is the brain the engine asks through. It records every request
// and reply. It does not implement the shape-retry seam, so each call goes out bare.
// A case that retried would certify the answer a model gives after being told to
// try again.
type verdictRecorder struct {
	completer aitasks.Completer
	requests  []model.Request
	replies   []string
	// failed is the completer's own failure. It stays apart from an answer the engine
	// refused, because a call that never completed is the lane's problem and no
	// measurement of the reply.
	failed error
}

func (r *verdictRecorder) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	r.requests = append(r.requests, req)
	resp, err := r.completer.Complete(ctx, req)
	if err != nil {
		r.failed = err
		return model.Response{}, err
	}
	r.replies = append(r.replies, resp.Text)
	return resp, nil
}

// Evaluate reads what the engine did. An unreadable reply, or an answer the engine
// refused to act on, is invalid. A sender retired to a human is wrong by the verdict
// it did not reach. A sender applied under another kind is wrong and a leak, since
// the kind decides who sees the sender. The floors count, since they are part of
// what the product serves.
func (c *counterpartyVerdictCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var run verdictRun
	if err := json.Unmarshal([]byte(trace.Output), &run); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("the run was not recorded: %v", err)}
	}
	switch {
	case run.Unusable != "":
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: run.Unusable}
	case run.Refused != "":
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: "the engine would not act on the answer: " + run.Refused}
	case run.Retired != "":
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the engine retired the sender to a human (%s) where the scenario expects %q", run.Retired, c.expected),
		}
	case run.Settled != c.expected:
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the engine applied %q where the scenario expects %q", run.Settled, c.expected),
			// The kind decides who sees the sender, so a wrong one is a leak.
			Leak: true,
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// counterpartyVerdictCase also has a decision form: the same sender asked of a
// decision model, read by the same gate the engine applies.
var _ aitasks.DecisionCase = (*counterpartyVerdictCase)(nil)

// DecisionSite is the site the engine asks its decision at.
func (*counterpartyVerdictCase) DecisionSite() string { return counterpartyDecisionSite }

// DecisionRequest is the engine's decision request, built from the same row
// Run builds the LLM request from.
func (c *counterpartyVerdictCase) DecisionRequest() decision.Request {
	return counterpartyDecision(c.row)
}

// GateDecision is the engine's own gate, so a certified answer is one the
// shipped path would have kept.
func (*counterpartyVerdictCase) GateDecision(a decision.Answer) ai.DecisionVerdict {
	return counterpartyDecisionGate(a)
}

// Floors are each kind's own floor, the one clearsItsFloor applies to it.
func (*counterpartyVerdictCase) Floors() map[string]float64 {
	kinds := verdictKindNames()
	floors := make(map[string]float64, len(kinds))
	for _, kind := range kinds {
		floors[kind] = verdictFloorFor(kind)
	}
	return floors
}

// EvaluateDecision grades an answer the gate kept. The answer is a closed
// label, so exact match against the scenario is the whole judgment.
func (c *counterpartyVerdictCase) EvaluateDecision(a decision.Answer) aitasks.Outcome {
	if a.Choice != c.expected {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the decision model answered %q at confidence %.2f where the scenario expects %q",
				a.Choice, a.Confidence, c.expected),
			Leak: true,
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
