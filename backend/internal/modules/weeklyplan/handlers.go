// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"encoding/json"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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

// The two literals only this file needs: the wire's null, and the code every
// malformed value here answers with. The field names and the refusal codes the
// STORE shares with its callers live in store.go.
const (
	jsonNull    = "null"
	codeInvalid = "invalid"
)

// EditWeeklyPlanCommitment corrects one of the caller's own commitments.
//
// DECODED AS RAW KEYS, not into the generated struct, and that is forced by the
// shape of the request. Both `due_on` absent and `due_on: null` render as a nil
// pointer there, and the two mean opposite things: leave the date alone, and
// clear it. A rep who fixes a typo would otherwise silently lose the date they
// never mentioned.
func (h Handlers) EditWeeklyPlanCommitment(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body map[string]json.RawMessage
	if !httperr.Decode(w, r, &body) {
		return
	}
	edit, err := editFromBody(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := h.store.EditCommitment(r.Context(), ids.UUID(id), edit); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// editFromBody reads the keys that are PRESENT into the store's edit.
//
// Each block is the same three steps — the key is there, it parses, it becomes
// a pointer — and the ParseError names the field the client sent rather than
// the Go one, so a malformed date reads as `due_on` and not as `DueOn`.
func editFromBody(body map[string]json.RawMessage) (CommitmentEdit, error) {
	var edit CommitmentEdit
	if raw, ok := body["label"]; ok {
		var label string
		if err := json.Unmarshal(raw, &label); err != nil {
			return edit, &values.ParseError{
				Field: fieldLabel, Code: codeInvalid, Message: "a label is text",
			}
		}
		edit.Label = &label
	}
	if raw, ok := body["due_on"]; ok {
		due, _, err := parseEditDate(raw)
		if err != nil {
			return edit, err
		}
		edit.DueOn = &due
	}
	if raw, ok := body["linked_record"]; ok {
		linkType, linkID, err := parseEditLink(raw)
		if err != nil {
			return edit, err
		}
		edit.LinkedRecordType, edit.LinkedRecordID = &linkType, &linkID
	}
	return edit, nil
}

// parseEditDate reads a date, or an explicit null meaning "clear it".
//
// A nil day with a nil error is the ANSWER here, not a missing one: JSON null
// on this field is a request to clear the date. Reported as a separate `ok`
// rather than as (nil, nil), which reads at a call site as a value nobody
// checked.
func parseEditDate(raw json.RawMessage) (*time.Time, bool, error) {
	if string(raw) == jsonNull {
		return nil, true, nil
	}
	badDate := &values.ParseError{
		Field: "due_on", Code: codeInvalid, Message: "a due date is a calendar day",
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, false, badDate
	}
	day, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return nil, false, badDate
	}
	return &day, true, nil
}

// parseEditLink reads a record link or an explicit null.
//
// A null unlinks, which reaches the store as the empty pair — the same shape
// the store already treats as "this commitment is about no record".
func parseEditLink(raw json.RawMessage) (string, ids.UUID, error) {
	if string(raw) == jsonNull {
		return "", ids.Nil, nil
	}
	var link crmcontracts.WeeklyPlanLink
	if err := json.Unmarshal(raw, &link); err != nil {
		return "", ids.Nil, &values.ParseError{
			Field: "linked_record", Code: codeInvalid,
			Message: "a linked record is a type and an id",
		}
	}
	return string(link.Type), ids.UUID(link.Id), nil
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
