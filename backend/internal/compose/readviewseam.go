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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// read_view serves a REST read by running that route's own handler, in
// process, as the caller: one engine, so the two doors cannot answer apart.

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

// readViews lists the views the contract declares: every route whose x-mcp-tool
// names read_view.
func readViews() []agents.ReadView {
	var views []agents.ReadView
	for route, pol := range agentPolicies {
		if pol.Tool != "read_view" {
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
	for _, prefix := range []string{"get", "list"} {
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
		req := httptest.NewRequestWithContext(context.WithValue(ctx, admittedByTheToolKey{}, true), http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		door.api.ServeHTTP(rec, req)
		return viewAnswer(rec.Result())
	}
}

// viewAnswer turns the route's answer into the tool's. A JSON body passes as it
// is, any other body is wrapped with its type, and a refusal is its error.
func viewAnswer(res *http.Response) (json.RawMessage, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, apperrors.ErrNotFound
	case res.StatusCode == http.StatusForbidden:
		return nil, apperrors.ErrPermissionDenied
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
	return json.Marshal(map[string]string{"content_type": res.Header.Get("Content-Type"), "body": string(body)})
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
	if features.served != nil {
		provider = provider.WithRetiredRendering(features.served.deleteBlob)
	}
	return provider
}
