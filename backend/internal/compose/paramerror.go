// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/queryguard"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// contractRouter builds the generated /v1 router. Every parameter refusal sits
// innermost, so a caller refused for who they are hears that before they hear
// their query was wrong.
//
// It returns the chi router itself, so the body ceiling can ask which route a path
// resolves to before the router serves it.
func contractRouter(srv crmcontracts.ServerInterface, outer ...crmcontracts.MiddlewareFunc) *chi.Mux {
	router := chi.NewRouter()
	router.Use(compressAPIJSON)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httperr.Write(w, r, apperrors.ErrNotFound)
	})
	router.MethodNotAllowed(methodNotAllowed(router))
	middlewares := append([]crmcontracts.MiddlewareFunc{queryguard.Refuse(chiRoutePattern), refuseUnknownQueryEnum, refuseQueryBelowMinimum}, outer...)
	crmcontracts.HandlerWithOptions(srv, crmcontracts.ChiServerOptions{
		BaseURL:     httpserver.BaseURL,
		BaseRouter:  router,
		Middlewares: append(middlewares, announceDeprecatedOperation, fileRefusalPage),
		// The generated default writes err.Error() as text/plain, an
		// off-contract shape that also leaks the parser's internal text.
		ErrorHandlerFunc: paramParseError,
	})
	return router
}

// paramParseError maps a generated request-parameter parse failure onto
// the same 422 validation_error shape every other bad query input uses
// (mirrors httperr's malformed-cursor path). It names only the offending
// parameter — never the wrapped parser text, which can carry internal
// detail — so a bad cursor/limit/sort/UUID answers problem+json, not a
// text/plain leak.
func paramParseError(w http.ResponseWriter, r *http.Request, err error) {
	setDeprecationHeaders(w, r)
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

// routedMethods is the order an Allow header lists the methods a path answers.
var routedMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// methodNotAllowed answers a path the router knows under another method. A
// custom handler is not handed chi's own list, so it asks the router again.
func methodNotAllowed(router *chi.Mux) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}
		var allowed []string
		for _, method := range routedMethods {
			if router.Match(chi.NewRouteContext(), method, path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusMethodNotAllowed,
			Code:   "method_not_allowed",
			Detail: "This path does not answer this method. The Allow header lists the ones it does.",
		})
	}
}
