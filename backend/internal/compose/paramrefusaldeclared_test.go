// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A route that refuses a malformed parameter with 422 declares that 422. The
// refusals are not read off the parameter types: the real router is asked, one
// bad parameter at a time. A parameter kind the binding learns to refuse joins
// the census by being refused.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// malformedValue parses as no uuid, date, number, boolean or enum member.
const malformedValue = "not-a-value"

// routedOperation is one operation with the path's shared parameters folded
// in; an operation-level parameter replaces a shared one of the same name.
type routedOperation struct {
	method, path string
	op           *openapi3.Operation
	params       []*openapi3.Parameter
}

func (o routedOperation) String() string { return o.method + " " + o.path }

func TestEveryOperationThatRefusesAParameterDeclaresTheRefusal(t *testing.T) {
	router := contractRouter(crmcontracts.Unimplemented{})
	refusing := 0
	for _, op := range routedOperations(t) {
		if code := op.serve(t, router, "", nil); code != http.StatusNotImplemented {
			t.Errorf("%s answered %d to parameters the contract admits, want the handler's 501: "+
				"this test's well-formed value for one of them is not one", op, code)
			continue
		}
		refused := op.refusedParameters(t, router)
		if len(refused) == 0 {
			continue
		}
		refusing++
		if op.op.Responses.Status(http.StatusUnprocessableEntity) == nil {
			t.Errorf("%s answers 422 to a malformed or missing %s but declares no 422; add "+
				"'422': { $ref: '#/components/responses/ValidationError' } to its responses",
				op, strings.Join(refused, ", "))
		}
	}
	// The floor sits below the real count, to catch a router or loader that
	// stopped answering rather than a contract that changed.
	if refusing < 400 {
		t.Fatalf("only %d operations refused a parameter; the census stopped reading the contract", refusing)
	}
}

// refusedParameters names each parameter whose malformed value, or absence
// when it is required, the router answers with 422.
func (o routedOperation) refusedParameters(t *testing.T, router http.Handler) []string {
	t.Helper()
	var refused []string
	for _, p := range o.params {
		code := o.serve(t, router, p.Name, &malformedParam{value: malformedValue})
		if code != http.StatusUnprocessableEntity && p.Required && p.In != openapi3.ParameterInPath {
			code = o.serve(t, router, p.Name, &malformedParam{omit: true})
		}
		if code == http.StatusUnprocessableEntity {
			refused = append(refused, p.In+" "+p.Name)
		}
	}
	return refused
}

// malformedParam replaces one parameter's well-formed value, or leaves it out.
type malformedParam struct {
	value string
	omit  bool
}

// serve sends the operation every required parameter well formed, with the
// named one replaced by bad, and answers the status.
func (o routedOperation) serve(t *testing.T, router http.Handler, name string, bad *malformedParam) int {
	t.Helper()
	path, query, header := o.path, url.Values{}, http.Header{}
	for _, p := range o.params {
		value, send := wellFormed(p.Schema.Value), p.Required
		if p.Name == name && bad != nil {
			value, send = bad.value, !bad.omit
		}
		if !send {
			continue
		}
		switch p.In {
		case openapi3.ParameterInPath:
			path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(value))
		case openapi3.ParameterInQuery:
			query.Set(p.Name, value)
		case openapi3.ParameterInHeader:
			header.Set(p.Name, value)
		default:
			t.Fatalf("%s: parameter %s is in %s, which this test does not send", o, p.Name, p.In)
		}
	}
	req := httptest.NewRequest(o.method, httpserver.BaseURL+path+"?"+query.Encode(), nil)
	req.Header = header
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder.Code
}

// wellFormed answers a value the contract admits for the schema.
func wellFormed(schema *openapi3.Schema) string {
	switch {
	case len(schema.Enum) > 0:
		return fmt.Sprint(schema.Enum[0])
	case schema.Type.Is(openapi3.TypeArray) && schema.Items != nil:
		return wellFormed(schema.Items.Value)
	case schema.Format == "uuid":
		return ids.NewV7().String()
	case schema.Format == "date":
		return "2026-01-02"
	case schema.Format == "date-time":
		return "2026-01-02T03:04:05Z"
	case schema.Type.Is(openapi3.TypeInteger), schema.Type.Is(openapi3.TypeNumber):
		if schema.Min != nil {
			return strconv.FormatFloat(*schema.Min, 'f', -1, 64)
		}
		return "1"
	case schema.Type.Is(openapi3.TypeBoolean):
		return "true"
	default:
		return "placeholder"
	}
}

// routedOperations answers every operation of the contract, in path order.
func routedOperations(t *testing.T) []routedOperation {
	t.Helper()
	paths := loadContract(t).Paths.Map()
	var out []routedOperation
	for path, item := range paths {
		for method, op := range item.Operations() {
			byName := map[string]*openapi3.Parameter{}
			var order []string
			for _, ref := range append(append(openapi3.Parameters{}, item.Parameters...), op.Parameters...) {
				key := ref.Value.In + " " + ref.Value.Name
				if _, seen := byName[key]; !seen {
					order = append(order, key)
				}
				byName[key] = ref.Value
			}
			params := make([]*openapi3.Parameter, 0, len(order))
			for _, key := range order {
				params = append(params, byName[key])
			}
			out = append(out, routedOperation{method: method, path: path, op: op, params: params})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
