// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// orderVerb addresses one order by the id in its path, with the given method.
func orderVerb(method, idType string) extension.Verb {
	v := unitVerb("alpha", "read_order", extension.TierAutoExecute, extension.ScopeRead)
	v.Method = method
	v.Route = "/ext/alpha/orders/{order_id}"
	v.InputSchema = json.RawMessage(`{"type":"object","properties":{"order_id":{"type":"` + idType + `"},` +
		`"note":{"type":"string"}},"required":["order_id"]}`)
	return v
}

// The id in the path reaches the tool as an argument of its declared type, the
// same argument an agent passes through MCP.
func TestAPathArgumentReachesTheToolAsAnArgument(t *testing.T) {
	for name, tc := range map[string]struct {
		verb      extension.Verb
		method    string
		url, body string
		want      string
	}{
		"a GET":                {orderVerb(http.MethodGet, "string"), http.MethodGet, "/v1/ext/alpha/orders/abc?note=n", "", `{"note":"n","order_id":"abc"}`},
		"a POST":               {orderVerb(http.MethodPost, "string"), http.MethodPost, "/v1/ext/alpha/orders/abc", `{"note":"n"}`, `{"note":"n","order_id":"abc"}`},
		"a POST with no body":  {orderVerb(http.MethodPost, "string"), http.MethodPost, "/v1/ext/alpha/orders/abc", "", `{"order_id":"abc"}`},
		"an integer id":        {orderVerb(http.MethodGet, "integer"), http.MethodGet, "/v1/ext/alpha/orders/42", "", `{"order_id":42}`},
		"an escaped character": {orderVerb(http.MethodGet, "string"), http.MethodGet, "/v1/ext/alpha/orders/a%2Fb", "", `{"order_id":"a/b"}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			echoArgs(t, tc.verb).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body)))
			if rec.Code != http.StatusOK || rec.Body.String() != tc.want {
				t.Fatalf("status %d, arguments %s; want 200 and %s", rec.Code, rec.Body, tc.want)
			}
		})
	}
}

// An id sent twice, or of the wrong type, is refused rather than resolved.
func TestAPathArgumentTheRouteCannotUseIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		verb      extension.Verb
		method    string
		url, body string
	}{
		"also in the query":  {orderVerb(http.MethodGet, "string"), http.MethodGet, "/v1/ext/alpha/orders/abc?order_id=def", ""},
		"also in the body":   {orderVerb(http.MethodPost, "string"), http.MethodPost, "/v1/ext/alpha/orders/abc", `{"order_id":"def"}`},
		"a body not object":  {orderVerb(http.MethodPost, "string"), http.MethodPost, "/v1/ext/alpha/orders/abc", `["x"]`},
		"not an integer":     {orderVerb(http.MethodGet, "integer"), http.MethodGet, "/v1/ext/alpha/orders/abc", ""},
		"not an integer too": {orderVerb(http.MethodPost, "integer"), http.MethodPost, "/v1/ext/alpha/orders/abc", ""},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			echoArgs(t, tc.verb).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body)))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

// ServeMux panics on two routes that differ only in their argument names. The
// mount refuses them first and names the units.
func TestTwoRoutesOfOneShapeAreRefusedAtMount(t *testing.T) {
	a := orderVerb(http.MethodGet, "string")
	b := orderVerb(http.MethodGet, "string")
	b.Tool, b.OperationID = "read_order_by_ref", "readOrderByRefOp"
	b.Route = "/ext/alpha/orders/{ref}"
	b.InputSchema = json.RawMessage(`{"type":"object","properties":{"ref":{"type":"string"}},"required":["ref"]}`)
	_, err := MountExtensionRoutes(http.NewServeMux(), []extension.Verb{a, b}, nil, noopInvoker)
	if err == nil || !strings.Contains(err.Error(), "both declare") {
		t.Fatalf("err = %v, want the same-shape refusal", err)
	}
}
