// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	chi "github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// read_view serves a REST read by running that route's own handler, in
// process, as the caller: one engine, so the two doors cannot answer apart.

// viewQuery is how the generated table spells a view's query parameter.
type viewQuery = agents.ViewQuery

// viewDoor holds the contract router once the server has built it. The tool
// registry is built first, so the seam reaches the router through this.
type viewDoor struct {
	api http.Handler
}

// admittedByTheToolKey marks a request read_view runs. The MCP door already
// admitted the call (scope, seat, volume), so the REST gate must not admit and
// charge it again. It is a context value, which no client can send.
type admittedByTheToolKey struct{}

func admittedByTheTool(r *http.Request) bool {
	admitted, _ := r.Context().Value(admittedByTheToolKey{}).(bool)
	return admitted
}

var pathParameter = regexp.MustCompile(`\{([^}]+)\}`)

// readViews lists the views the contract declares: every agent GET route whose
// x-mcp-tool names read_view. A human-only or writing route is never a view.
func readViews(policies map[string]agentPolicy) []agents.ReadView {
	var views []agents.ReadView
	for route, pol := range policies {
		if pol.Tool != agents.ReadViewTool || pol.Access != accessTool || !strings.HasPrefix(route, http.MethodGet+" ") {
			continue
		}
		pattern := strings.SplitN(route, " ", 2)[1]
		var params []string
		for _, m := range pathParameter.FindAllStringSubmatch(pattern, -1) {
			params = append(params, m[1])
		}
		views = append(views, agents.ReadView{
			Name: viewName(pol.Op), Op: pol.Op, Pattern: pattern, PathParams: params, Query: readViewQueries[route],
		})
	}
	slices.SortFunc(views, func(a, b agents.ReadView) int { return strings.Compare(a.Name, b.Name) })
	return views
}

// viewName spells an operation as a caller asks for it: getCompany360 is
// company_360 and listLeadSources is lead_sources.
func viewName(op string) string {
	for _, prefix := range []string{"get", "list"} { //nolint:goconst // an operationId's verb prefixes, not record types
		if rest, ok := strings.CutPrefix(op, prefix); ok && rest != "" && unicode.IsUpper(rune(rest[0])) {
			op = rest
			break
		}
	}
	runes := []rune(op)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && wordStarts(runes, i) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// wordStarts reports whether runes[i] opens a word. A capital after a small
// letter does, and so does a digit after a letter. In a run of capitals, the
// last one before a small letter does: OpenAPIDocument is open_api_document.
func wordStarts(runes []rune, i int) bool {
	prev, r := runes[i-1], runes[i]
	switch {
	case unicode.IsDigit(r):
		return !unicode.IsDigit(prev)
	case !unicode.IsUpper(r):
		return false
	case unicode.IsLower(prev) || unicode.IsDigit(prev):
		return true
	default:
		return i+1 < len(runes) && unicode.IsLower(runes[i+1])
	}
}

// readViewThrough runs a view's route through the contract router.
func readViewThrough(door *viewDoor) agents.ViewReader {
	return func(ctx context.Context, view agents.ReadView, path map[string]string, query url.Values) (json.RawMessage, error) {
		if door == nil || door.api == nil {
			return nil, fmt.Errorf("read_view: views are served by the API process: %w", apperrors.ErrUnsupportedBySoR)
		}
		target := pathParameter.ReplaceAllStringFunc(view.Pattern, func(m string) string {
			return url.PathEscape(path[strings.Trim(m, "{}")])
		})
		if len(query) > 0 {
			target += "?" + query.Encode()
		}
		// A fresh chi route context, so an outer chi request's matched route and
		// URL parameters never carry into this one.
		inner := context.WithValue(context.WithValue(ctx, admittedByTheToolKey{}, true), chi.RouteCtxKey, (*chi.Context)(nil))
		req := httptest.NewRequestWithContext(inner, http.MethodGet, target, nil)
		rec := &cappedRecorder{ResponseRecorder: httptest.NewRecorder()}
		door.api.ServeHTTP(rec, req)
		if rec.overflowed {
			return nil, httperr.Validation("view", "view_too_large", fmt.Sprintf(
				"the view answered more than %d MB; narrow it with its filters, or fetch it from its REST route",
				maxViewAnswerBytes>>20))
		}
		return viewAnswer(rec.Result())
	}
}

// maxViewAnswerBytes bounds what one view may hold in memory. An answer past
// it, such as a whole CSV export, is no use to a model either.
const maxViewAnswerBytes = 4 << 20

// cappedRecorder keeps at most maxViewAnswerBytes of a route's body and notes
// what it dropped. The handler runs to its end on bounded memory.
type cappedRecorder struct {
	*httptest.ResponseRecorder
	overflowed bool
}

func (c *cappedRecorder) Write(p []byte) (int, error) {
	if c.overflowed || c.Body.Len()+len(p) > maxViewAnswerBytes {
		c.overflowed = true
		return len(p), nil
	}
	return c.ResponseRecorder.Write(p)
}

// viewRefusals are the route answers that mean the same on the tool's side. A
// refusal keeps its kind instead of reading as the caller's bad argument.
var viewRefusals = map[int]error{
	http.StatusUnauthorized:       apperrors.ErrPermissionDenied,
	http.StatusForbidden:          apperrors.ErrPermissionDenied,
	http.StatusNotFound:           apperrors.ErrNotFound,
	http.StatusConflict:           apperrors.ErrConflict,
	http.StatusPreconditionFailed: apperrors.ErrVersionSkew,
	http.StatusTooManyRequests:    apperrors.ErrBudgetExceeded,
}

// viewAnswer turns the route's answer into the tool's. A JSON body passes as it
// is, any other body is wrapped with its type, and a refusal is its error.
func viewAnswer(res *http.Response) (json.RawMessage, error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if refusal, known := viewRefusals[res.StatusCode]; known {
		return nil, refusal
	}
	switch {
	case res.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("read_view: the route failed with %d", res.StatusCode)
	case res.StatusCode >= http.StatusBadRequest:
		var problem struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		}
		if err := json.Unmarshal(body, &problem); err != nil || problem.Detail == "" {
			return nil, fmt.Errorf("read_view: the route answered %d: %w", res.StatusCode, apperrors.ErrInvalidArgument)
		}
		return nil, httperr.Validation("view", problem.Code, problem.Detail)
	}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") && len(body) > 0 {
		if body[0] == '[' {
			return json.Marshal(map[string]json.RawMessage{"items": body})
		}
		return body, nil
	}
	return json.Marshal(struct {
		ContentType string `json:"content_type"`
		Text        string `json:"text"`
	}{res.Header.Get("Content-Type"), string(body)})
}

// viewDoorOf is the serving process's router door, or none for a registry
// built without one.
func viewDoorOf(served *Server) *viewDoor {
	if served == nil {
		return nil
	}
	return served.views
}

// withServerSeams wires the provider to what the serving process holds: list
// membership while lists are on, and the object store an offer change reclaims.
func withServerSeams(provider *Provider, pool *pgxpool.Pool, features registryFeatures) *Provider {
	if features.lists {
		provider = provider.WithListMembers(NewCollectionsStore(pool).MemberFilter)
	}
	switch {
	case features.served != nil:
		provider = provider.WithRetiredRendering(features.served.deleteBlob)
	case features.reclaim != nil:
		provider = provider.WithRetiredRendering(features.reclaim)
	}
	return provider
}
