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
// after their behaviour was retired, so a client reads a named refusal and not a 404.
const retiredRouteDetail = "this endpoint is retired and its behaviour is gone"

func (Server) RevertStageProgression(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) GetStageAutomationReport(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) ListTransitionPolicies(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) SetTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) ResumeTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) GetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) SetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) WaitInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) CloseLostInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}

func (Server) GetDealViability(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplementedBecause(w, r, retiredRouteDetail)
}
