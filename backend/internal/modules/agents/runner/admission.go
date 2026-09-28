// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// AdmissionRefused marks a step the gate or the allowlist refused. Exported
// because the certification case grades a refused first step as the step the
// turn took, and must read the same word this package writes.
const AdmissionRefused = "refused"

// observeRefusal feeds a refusal back as an observation and returns the
// trace step for it. A DECLARED capability gap is called out as terminal,
// because it is not a fault the model can route around by trying again — and
// this is the loop with a step budget, so a re-plan that re-calls the same
// tool spends the run on a permanent no.
func observeRefusal(win *window, step modelStep, err error, meta Meta, resp model.Response) Step {
	observation := "tool call refused: " + err.Error()
	// Telling the model not to retry is an ORDER, so it rides the directive
	// argument: inside the fence it would be text the prompt has already
	// declared to be data the model must disregard (observeThen's own doc). The
	// trace keeps both halves joined — a trace is a record of what happened,
	// not a prompt.
	directive := ""
	switch {
	case errors.Is(err, apperrors.ErrUnsupportedBySoR):
		directive = "this workspace's system of record cannot serve this tool at all; do not call it again in this run"
	case errors.Is(err, errOutsideAgentSpec):
		// Permanent for the same reason and for a different cause: the
		// allowlist is code, so no re-plan reaches it within this run.
		directive = "this tool is outside what this agent may do; do not call it again in this run"
	}
	win.observeThen(step.Tool, observation, directive)
	// Reserve the directive's room inside the cap: provider text whose LENGTH is
	// influenceable by mirrored content must neither crowd "this was terminal"
	// out of the trace nor grow the entry past the bound.
	suffix := ""
	if directive != "" {
		suffix = " — " + directive
	}
	recorded := truncateTo(observation, traceObservationLimit-len(suffix)) + suffix
	return Step{
		Tool: step.Tool, Args: step.Args, Observation: recorded,
		ModelID: meta.ModelID, Tier: meta.Tier, TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens,
		Admission: AdmissionRefused,
	}
}
