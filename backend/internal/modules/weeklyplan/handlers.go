// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the plan's HTTP surface.
//
// Human-only, every operation. A plan is what a person means to do and what
// they say they are stuck on; an agent writing one would be the product
// deciding a rep's week for them, and an agent reading one would put a
// colleague's admission of being behind into a context nobody chose.
type Handlers struct {
	store *Store
	now   func() time.Time
}

// NewHandlers binds the routes to the store.
func NewHandlers(store *Store, now func() time.Time) Handlers {
	return Handlers{store: store, now: now}
}

// GetCurrentWeeklyPlan answers the caller's plan for this week.
func (h Handlers) GetCurrentWeeklyPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := h.store.Current(r.Context(), h.now())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, planToWire(plan))
}

// StartWeeklyPlan opens the caller's plan for this week.
func (h Handlers) StartWeeklyPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := h.store.StartWeek(r.Context(), h.now())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, planToWire(plan))
}

// GetTeammateWeeklyPlan answers a named rep's plan, for their lead.
func (h Handlers) GetTeammateWeeklyPlan(
	w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID,
) {
	plan, err := h.store.PlanFor(r.Context(), ids.UUID(ownerID), h.now())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, planToWire(plan))
}

// AddWeeklyPlanCommitment writes one commitment onto the caller's own plan.
func (h Handlers) AddWeeklyPlanCommitment(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.NewWeeklyPlanCommitment
	if !httperr.Decode(w, r, &body) {
		return
	}
	in := NewCommitment{Label: body.Label}
	if body.LinkedRecord != nil {
		in.LinkedRecordType = string(body.LinkedRecord.Type)
		in.LinkedRecordID = ids.UUID(body.LinkedRecord.Id)
	}
	if body.DueOn != nil {
		due := body.DueOn.Time
		in.DueOn = &due
	}
	out, err := h.store.AddCommitment(r.Context(), h.now(), in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, commitmentToWire(out))
}

// SetWeeklyPlanCommitmentState settles one of the caller's commitments.
func (h Handlers) SetWeeklyPlanCommitmentState(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body struct {
		State string `json:"state"`
	}
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.SetState(r.Context(), ids.UUID(id), body.State); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AskForWeeklyPlanHelp records what the caller needs on one commitment.
func (h Handlers) AskForWeeklyPlanHelp(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body struct {
		HelpRequested string `json:"help_requested"`
	}
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.AskForHelp(r.Context(), ids.UUID(id), body.HelpRequested); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AnswerWeeklyPlanCommitment records the lead's answer on one commitment.
func (h Handlers) AnswerWeeklyPlanCommitment(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body struct {
		ManagerResponse string `json:"manager_response"`
	}
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.Respond(r.Context(), ids.UUID(id), body.ManagerResponse); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// planToWire puts one plan on the contract's shape.
func planToWire(plan Plan) crmcontracts.WeeklyPlan {
	commitments := make([]crmcontracts.WeeklyPlanCommitment, 0, len(plan.Commitments))
	for _, c := range plan.Commitments {
		commitments = append(commitments, commitmentToWire(c))
	}
	return crmcontracts.WeeklyPlan{
		Id:             openapi_types.UUID(plan.ID),
		LocalWeekStart: openapi_types.Date{Time: plan.LocalWeekStart},
		Status:         crmcontracts.WeeklyPlanStatus(plan.Status),
		Commitments:    commitments,
	}
}

// commitmentToWire puts one commitment on the contract's shape.
//
// The prose fields are ABSENT rather than empty when nothing was said: a reader
// cannot tell an empty string meaning "asked nothing" from one meaning "asked
// and the text was lost", and only one of those is ever true here.
func commitmentToWire(c Commitment) crmcontracts.WeeklyPlanCommitment {
	out := crmcontracts.WeeklyPlanCommitment{
		Id:              openapi_types.UUID(c.ID),
		Label:           c.Label,
		State:           crmcontracts.WeeklyPlanCommitmentState(c.State),
		Position:        c.Position,
		CompletedAt:     c.CompletedAt,
		RespondedAt:     c.RespondedAt,
		HelpRequested:   nullableText(c.HelpRequested),
		ManagerResponse: nullableText(c.ManagerResponse),
	}
	if c.LinkedRecordType != "" {
		out.LinkedRecord = &crmcontracts.WeeklyPlanLink{
			Type: crmcontracts.WeeklyPlanLinkType(c.LinkedRecordType),
			Id:   openapi_types.UUID(c.LinkedRecordID),
		}
	}
	if c.DueOn != nil {
		out.DueOn = &openapi_types.Date{Time: *c.DueOn}
	}
	if c.ManagerUserID != nil {
		manager := openapi_types.UUID(*c.ManagerUserID)
		out.ManagerUserId = &manager
	}
	return out
}

// nullableText renders an unsaid thing as absent rather than as an empty string.
func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
