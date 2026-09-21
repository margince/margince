// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The admin surface for what a transition may do.
//
// Three verbs, kept apart on purpose. Reading the rules and saving one are the
// ordinary pair; lifting a suspension is its own POST, so turning a safety stop
// off is something somebody does deliberately rather than a side effect of
// editing a threshold.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListTransitionPolicies answers every rule on one pipeline.
func (h Handlers) ListTransitionPolicies(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
) {
	rules, err := h.store.ReadTransitionPolicies(
		r.Context(), pathID[ids.PipelineKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := make([]crmcontracts.TransitionPolicy, 0, len(rules))
	for _, rule := range rules {
		out = append(out, transitionPolicyWire(rule))
	}
	httperr.WriteJSON(w, http.StatusOK,
		crmcontracts.TransitionPolicyList{Data: out})
}

// SetTransitionPolicy records what an admin wants for one transition.
func (h Handlers) SetTransitionPolicy(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
	_ crmcontracts.SetTransitionPolicyParams,
) {
	var body crmcontracts.SetTransitionPolicyRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	ref, err := transitionRefFromBody(
		pathID[ids.PipelineKind](id), body.FromStageId, body.ToStageId)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The contract's own bounds, enforced here because nothing else does. The
	// generated Valid() is not called by the router, and the column CHECKs
	// only test positivity — so a mode the contract does not define reached
	// the store and came back a 500, and a window of 400 days was stored with
	// a 200 while the contract says 365.
	if err := checkPolicyBounds(body); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	saved, err := h.store.SetTransitionPolicy(r.Context(), SetTransitionPolicyInput{
		TransitionRef:               ref,
		Mode:                        AutopilotMode(body.Mode),
		CleanAcceptanceThreshold:    body.CleanAcceptanceThreshold,
		CorrectionReversalThreshold: body.CorrectionReversalThreshold,
		MinReviewed:                 body.MinReviewed,
		MinObservationDays:          body.MinObservationDays,
		WindowDays:                  body.WindowDays,
		UndoWindowHours:             body.UndoWindowHours,
		IfVersion:                   body.IfVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, transitionPolicyWire(saved))
}

// ResumeTransitionPolicy lifts a suspension the product put on a transition.
//
// It answers the rule as it now stands rather than 204, because what an admin
// needs to see next is what the rule came back TO: resuming does not
// re-enable, so a rule that was on auto is on auto again and still has to
// clear its thresholds before anything applies.
func (h Handlers) ResumeTransitionPolicy(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
) {
	var body crmcontracts.TransitionRef
	if !httperr.Decode(w, r, &body) {
		return
	}
	ref, err := transitionRefFromBody(
		pathID[ids.PipelineKind](id), body.FromStageId, body.ToStageId)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The row the resume TRANSACTION committed, not a second read of it.
	// Re-reading needs its own pipeline:read, so an update-only caller could
	// clear a suspension and then be told 403 — a completed mutation looking
	// like a failed one. It also races: a concurrent suspension between the
	// two calls would answer a re-suspended row under a 200 that says the
	// rule is running.
	resumed, err := h.store.ResumeTransitionPolicy(r.Context(), ref)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, transitionPolicyWire(resumed))
}

// checkPolicyBounds holds a saved rule to what the contract promises.
//
// Every bound here is published — the enum's members, the maxima on the two
// windows — and a caller who reads the contract is entitled to a 422 naming
// the field rather than a 500 or a silent acceptance. The database is not the
// place for these: its CHECKs test positivity, which is the invariant the
// TABLE needs, not the one the API states.
func checkPolicyBounds(body crmcontracts.SetTransitionPolicyRequest) error {
	if !body.Mode.Valid() {
		return httperr.Validation("mode", "unknown_mode",
			"a transition either proposes its moves or applies them; there is no third answer")
	}
	if body.WindowDays != nil && (*body.WindowDays < 1 || *body.WindowDays > 365) {
		return httperr.Validation("window_days", "out_of_range",
			"a rate is counted over 1 to 365 days")
	}
	if body.UndoWindowHours != nil &&
		(*body.UndoWindowHours < 1 || *body.UndoWindowHours > 8760) {
		return httperr.Validation("undo_window_hours", "out_of_range",
			"an undo window runs from 1 hour to a year")
	}
	if err := checkRate("clean_acceptance_threshold", body.CleanAcceptanceThreshold); err != nil {
		return err
	}
	return checkRate("correction_reversal_threshold", body.CorrectionReversalThreshold)
}

// checkRate refuses a threshold outside 0..1.
//
// Both are shares of reviewed proposals, so a value above 1 is a caller who
// has written a percentage where a fraction belongs — and stored, it would
// make a transition that can never clear its own bar look simply unlucky.
func checkRate(field string, rate *float64) error {
	if rate == nil || (*rate >= 0 && *rate <= 1) {
		return nil
	}
	return httperr.Validation(field, "out_of_range",
		"a threshold is a share of reviewed proposals, between 0 and 1")
}

// transitionPolicyWire renders one rule.
func transitionPolicyWire(p TransitionPolicy) crmcontracts.TransitionPolicy {
	out := crmcontracts.TransitionPolicy{
		Id:                          openapi_types.UUID(p.ID),
		PipelineId:                  openapi_types.UUID(p.PipelineID.UUID),
		FromStageId:                 openapi_types.UUID(p.FromStageID.UUID),
		ToStageId:                   openapi_types.UUID(p.ToStageID.UUID),
		Mode:                        crmcontracts.TransitionPolicyMode(p.Mode),
		CleanAcceptanceThreshold:    p.CleanAcceptanceThreshold,
		CorrectionReversalThreshold: p.CorrectionReversalThreshold,
		MinReviewed:                 p.MinReviewed,
		MinObservationDays:          p.MinObservationDays,
		WindowDays:                  p.WindowDays,
		UndoWindowHours:             p.UndoWindowHours,
		EnabledAt:                   p.EnabledAt,
		SuspendedAt:                 p.SuspendedAt,
		SuspendedReason:             p.SuspendedReason,
		Version:                     p.Version,
	}
	if p.EnabledBy != nil {
		enabler := openapi_types.UUID(*p.EnabledBy)
		out.EnabledBy = &enabler
	}
	return out
}
