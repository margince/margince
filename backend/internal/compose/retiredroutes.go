// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// A retired operation stays declared. Its refusal must not read as a server fault.
var errRouteRetired = &httperr.DetailedError{
	Status: http.StatusGone,
	Code:   "endpoint_retired",
	Detail: "this endpoint is retired and its behaviour is gone",
}

func answerRetired(w http.ResponseWriter, r *http.Request) {
	httperr.Write(w, r, errRouteRetired)
}

func (Server) RevertStageProgression(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id, _ openapi_types.UUID) {
	answerRetired(w, r)
}

func (Server) GetStageAutomationReport(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

func (Server) ListTransitionPolicies(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

func (Server) SetTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

func (Server) ResumeTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

func (Server) GetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

func (Server) SetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

func (Server) WaitInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	answerRetired(w, r)
}

func (Server) CloseLostInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	answerRetired(w, r)
}

func (Server) GetDealViability(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

func (Server) GetMorningDigest(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}
