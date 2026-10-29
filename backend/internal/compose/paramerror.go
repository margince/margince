// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// contractRouter builds the generated /v1 router. Every parameter refusal sits
// innermost, so a caller refused for who they are hears that before they hear
// their query was wrong. Operation hooks sit inside even those, so the agent
// gate judges the body the caller sent and a unit's change reaches only core.
func contractRouter(srv crmcontracts.ServerInterface, outer ...crmcontracts.MiddlewareFunc) http.Handler {
	return crmcontracts.HandlerWithOptions(srv, crmcontracts.ChiServerOptions{
		BaseURL:     httpserver.BaseURL,
		Middlewares: append([]crmcontracts.MiddlewareFunc{operationHooks, refuseUnknownQueryEnum, refuseQueryBelowMinimum}, outer...),
		// The generated default writes err.Error() as text/plain, an
		// off-contract shape that also leaks the parser's internal text.
		ErrorHandlerFunc: paramParseError,
	})
}

// paramParseError maps a generated request-parameter parse failure onto
// the same 422 validation_error shape every other bad query input uses
// (mirrors httperr's malformed-cursor path). It names only the offending
// parameter — never the wrapped parser text, which can carry internal
// detail — so a bad cursor/limit/sort/UUID answers problem+json, not a
// text/plain leak.
func paramParseError(w http.ResponseWriter, r *http.Request, err error) {
	param := "request"
	switch e := err.(type) {
	case *crmcontracts.RequiredParamError:
		param = e.ParamName
	case *crmcontracts.InvalidParamFormatError:
		param = e.ParamName
	case *crmcontracts.TooManyValuesForParamError:
		param = e.ParamName
	case *crmcontracts.UnmarshalingParamError:
		param = e.ParamName
	case *crmcontracts.UnescapedCookieParamError:
		param = e.ParamName
	}
	httperr.Write(w, r, httperr.Validation(param, "invalid_parameter",
		"parameter is missing or malformed"))
}
