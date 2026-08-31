// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for capture_confidentiality_verdict/thread.
//
// It certifies the shipped path rather than a description of it: the request
// comes from confidentialityRequest, the same builder the engine calls, and the
// reply is judged by validateConfidentialityPayload, the same validator the
// engine applies. A case that rebuilt either would measure a copy, and a copy
// stays green through the change that breaks the original.
//
// What a run here means is asymmetric, exactly as the engine's floor is. A
// thread wrongly called `ordinary` is a founder's shareholder negotiation on a
// colleague's screen; a thread wrongly held costs somebody one click. So a
// corpus scenario that expects a holding kind and gets `ordinary` is the
// failure worth catching, and the reverse is a cost worth paying.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// confidentialityFixture is one thread, in exactly the fields the ledger row
// hands the engine. It carries no id — see Prepare.
type confidentialityFixture struct {
	Subject     string   `json:"subject"`
	Body        string   `json:"body"`
	Attachments []string `json:"attachments"`
}

// confidentialityCases serves the one site that judges a thread.
type confidentialityCases struct{}

func (confidentialityCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskCaptureConfidentialityVerdict,
		Variant: "thread",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope narrows the record to the one call this case makes.
func (confidentialityCases) CertifiedScope() string { return aitasks.ScopeSingleCall }

// Prepare turns one thread and the kind the scenario expects into a runnable
// case, MINTING the ledger row id rather than reading it from either.
//
// Production takes that id from the ledger row, which no model has ever seen,
// so the only way an answer can carry it is to have read it out of this call's
// prompt. A fixture that supplied the id would hand it to whoever authored the
// expected reply, and a model echoing back an id it was given would then be
// indistinguishable from one that answered about the right thread.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (confidentialityCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f confidentialityFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("capture_confidentiality_verdict/thread: the fixture is not the shape this site takes: %w", err)
	}
	var want string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"capture_confidentiality_verdict/thread: the expected answer is not a kind token: %w", err)
	}
	// An expectation outside the closed vocabulary is unreachable: the validator
	// refuses every reply that could satisfy it, so the scenario would measure
	// nothing for as long as it stayed in the corpus.
	if _, known := statusForConfidentiality(want); !known {
		return nil, fmt.Errorf(
			"capture_confidentiality_verdict/thread: the scenario expects %q, which is not a thread kind", want)
	}
	return &confidentialityCase{
		row: capture.PendingThread{
			ID:          ids.NewV7(),
			Subject:     f.Subject,
			Body:        f.Body,
			Attachments: f.Attachments,
		},
		expected: want,
	}, nil
}

// confidentialityCase is one fixture thread ready to be judged, closed over the
// minted row id and the kind the scenario expects.
type confidentialityCase struct {
	row      capture.PendingThread
	expected string
}

// Run issues the one request this site sends.
func (c *confidentialityCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := confidentialityRequest(c.row)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("capture_confidentiality_verdict/thread: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then validateConfidentialityPayload against the row that was asked about —
// and only then asks whether the answer is the one the scenario expects. The
// order is the meaning: a reply that fails the validator has no kind to
// disagree with.
func (c *confidentialityCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload confidentialityPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	if msg := validateConfidentialityPayload(payload, c.row); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	// validateConfidentialityPayload admits exactly one result: every result
	// must carry the one requested id, none may repeat it, and it must be
	// present.
	answered := payload.Results[0].Verdict
	if answered != c.expected {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the model answered %q where the scenario expects %q", answered, c.expected),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
