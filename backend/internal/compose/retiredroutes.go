// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// retiredRouteDetail answers the paths the contract keeps declared for one release
// after their behaviour was retired. A client reads a named refusal and not a 404.
const retiredRouteDetail = "this endpoint is retired and its behaviour is gone"

// RevertStageProgression refuses reverting an automatic stage move.
func (Server) RevertStageProgression(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// GetStageAutomationReport refuses the report on automatic stage moves.
func (Server) GetStageAutomationReport(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// ListTransitionPolicies refuses listing the trust set on stage transitions.
func (Server) ListTransitionPolicies(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// SetTransitionPolicy refuses setting trust on a stage transition.
func (Server) SetTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// ResumeTransitionPolicy refuses resuming a paused stage transition.
func (Server) ResumeTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// GetForecastAssuranceEnforcement refuses reading the qualified forecast switch.
func (Server) GetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// SetForecastAssuranceEnforcement refuses changing the qualified forecast switch.
func (Server) SetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// WaitInputCheck refuses parking an input check on a review date.
func (Server) WaitInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// CloseLostInputCheck refuses closing a deal as lost from an input check.
func (Server) CloseLostInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

// GetDealViability refuses reading the viability review of a deal.
func (Server) GetDealViability(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}
