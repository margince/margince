// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// errRouteRetired answers an operation the contract keeps declared and deprecated
// after its behaviour was removed. 410 is a refusal a client can act on; a 5xx
// would read as a server fault.
var errRouteRetired = &httperr.DetailedError{
	Status: http.StatusGone,
	Code:   "endpoint_retired",
	Detail: "this endpoint is retired and its behaviour is gone",
}

func answerRetired(w http.ResponseWriter, r *http.Request) {
	httperr.Write(w, r, errRouteRetired)
}

// RevertStageProgression refuses reverting an automatic stage move.
func (Server) RevertStageProgression(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id, _ openapi_types.UUID) {
	answerRetired(w, r)
}

// GetStageAutomationReport refuses the report on automatic stage moves.
func (Server) GetStageAutomationReport(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

// ListTransitionPolicies refuses listing the trust set on stage transitions.
func (Server) ListTransitionPolicies(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

// SetTransitionPolicy refuses setting trust on a stage transition.
func (Server) SetTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

// ResumeTransitionPolicy refuses resuming a paused stage transition.
func (Server) ResumeTransitionPolicy(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

// GetForecastAssuranceEnforcement refuses reading the qualified forecast switch.
func (Server) GetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

// SetForecastAssuranceEnforcement refuses changing the qualified forecast switch.
func (Server) SetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}

// WaitInputCheck refuses parking an input check on a review date.
func (Server) WaitInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	answerRetired(w, r)
}

// CloseLostInputCheck refuses closing a deal as lost from an input check.
func (Server) CloseLostInputCheck(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	answerRetired(w, r)
}

// GetDealViability refuses reading the viability review of a deal.
func (Server) GetDealViability(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	answerRetired(w, r)
}

// GetMorningDigest refuses reading the nightly capture digest.
func (Server) GetMorningDigest(w http.ResponseWriter, r *http.Request) {
	answerRetired(w, r)
}
