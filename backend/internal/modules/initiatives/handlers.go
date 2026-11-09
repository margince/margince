// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"encoding/json"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Handlers is the HTTP transport over the store.
type Handlers struct{ store *Store }

// NewHandlers binds the transport to the store.
func NewHandlers(store *Store) Handlers { return Handlers{store: store} }

func (h Handlers) ListSalesInitiatives(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSalesInitiativesParams) {
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	out, err := h.store.List(r.Context(), status)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SalesInitiativeListResponse{Data: out})
}

func (h Handlers) CreateSalesInitiative(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SalesInitiativeInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	definition, err := definitionOf(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.Create(r.Context(), definition)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

func (h Handlers) GetSalesInitiative(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.store.Get(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) UpdateSalesInitiative(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateSalesInitiativeParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body crmcontracts.SalesInitiativeInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	definition, err := definitionOf(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.Update(r.Context(), ids.UUID(id), definition, version)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) LaunchSalesInitiative(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.LaunchSalesInitiativeParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	out, err := h.store.Launch(r.Context(), ids.UUID(id), version)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) ListSalesInitiativeMembers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListSalesInitiativeMembersParams) {
	var recordType *string
	if params.RecordType != nil {
		value := string(*params.RecordType)
		recordType = &value
	}
	out, err := h.store.Members(r.Context(), ids.UUID(id), recordType)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h Handlers) ReplaceSalesInitiativeMembers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.SalesInitiativeMembersInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.store.ReplaceMembers(r.Context(), ids.UUID(id), body.Picked, body.Excluded)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func definitionOf(body crmcontracts.SalesInitiativeInput) (Definition, error) {
	out := Definition{Name: body.Name, Description: body.Description, Objective: string(body.ObjectiveMetric), Target: body.TargetValue, Deadline: body.Deadline.Time, IncludeFuture: body.IncludeFuture, Reason: body.Reason}
	if body.OwnerId != nil {
		owner := ids.UUID(*body.OwnerId)
		out.OwnerID = &owner
	}
	var err error
	if out.LeadRule, err = predicateOf("lead_rule", body.LeadRule); err != nil {
		return out, err
	}
	out.DealRule, err = predicateOf("deal_rule", body.DealRule)
	return out, err
}

// predicateOf reads a rule as the predicate engine's tree. An empty object is
// no rule at all, which the editor sends for "no conditions".
func predicateOf(field string, rule *crmcontracts.SalesInitiativeRule) (*storekit.Predicate, error) {
	if rule == nil || len(*rule) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	var predicate storekit.Predicate
	if err := json.Unmarshal(raw, &predicate); err != nil {
		return nil, httperr.Validation(field, "invalid", "the rule is not a filter tree")
	}
	return &predicate, nil
}
