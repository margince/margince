// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A record write names the fields the record has, or a cf_ key. The schemas that
// take custom fields set additionalProperties, so the generated decoder takes any
// key. Both transports decode them through the split decode instead, which holds
// every other key to the closed rules. This gate reads the contract for every
// body that takes extra keys. It proves each is held on the web wire and the tool seam.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customFieldRequestTypes maps each request schema that takes custom fields to
// its generated type. The census below reads the contract, so a body added there
// and missing here fails.
var customFieldRequestTypes = map[string]reflect.Type{
	"CreateActivityRequest": reflect.TypeFor[crmcontracts.CreateActivityRequest](),
	"UpdateActivityRequest": reflect.TypeFor[crmcontracts.UpdateActivityRequest](),
	"CreateCompanyRequest":  reflect.TypeFor[crmcontracts.CreateCompanyRequest](),
	"UpdateCompanyRequest":  reflect.TypeFor[crmcontracts.UpdateCompanyRequest](),
	"CreateContactRequest":  reflect.TypeFor[crmcontracts.CreateContactRequest](),
	"UpdateContactRequest":  reflect.TypeFor[crmcontracts.UpdateContactRequest](),
	"CreateContractRequest": reflect.TypeFor[crmcontracts.CreateContractRequest](),
	"UpdateContractRequest": reflect.TypeFor[crmcontracts.UpdateContractRequest](),
	"RenewContractRequest":  reflect.TypeFor[crmcontracts.RenewContractRequest](),
	"CreateDealRequest":     reflect.TypeFor[crmcontracts.CreateDealRequest](),
	"UpdateDealRequest":     reflect.TypeFor[crmcontracts.UpdateDealRequest](),
	"CreateLeadRequest":     reflect.TypeFor[crmcontracts.CreateLeadRequest](),
	"UpdateLeadRequest":     reflect.TypeFor[crmcontracts.UpdateLeadRequest](),
	"CreateProjectRequest":  reflect.TypeFor[crmcontracts.CreateProjectRequest](),
	"UpdateProjectRequest":  reflect.TypeFor[crmcontracts.UpdateProjectRequest](),
}

// freeFormBodies are open bodies that write no record directly: the fields are
// a stranger's, read by name and kept as text. Each says what bounds it.
var freeFormBodies = gatekit.Waive(map[string]string{
	"WebFormFields": "a website form's own fields, read into lead fields by name; the rest become note text, bounded in count and length by webforms",
})

// openBodies collects every schema a body takes that admits extra keys. It looks
// through a $ref, an allOf, oneOf or anyOf member, and an inline schema. An inline
// one is named by its operation, so it fails the census instead of passing unseen.
func openBodies(doc map[string]any, schema map[string]any, inline string, seen map[string]bool) []string {
	if ref, ok := schema["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		resolved, found := resolveSchemaRef(doc, ref)
		if !found || seen[name] {
			return nil
		}
		seen[name] = true
		var out []string
		if resolved["additionalProperties"] == true {
			out = append(out, name)
		}
		return append(out, composed(doc, resolved, inline, seen)...)
	}
	var out []string
	if schema["additionalProperties"] == true {
		out = append(out, inline)
	}
	return append(out, composed(doc, schema, inline, seen)...)
}

func composed(doc map[string]any, schema map[string]any, inline string, seen map[string]bool) []string {
	var out []string
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		members, _ := schema[keyword].([]any)
		for _, member := range members {
			if node, ok := member.(map[string]any); ok {
				out = append(out, openBodies(doc, node, inline, seen)...)
			}
		}
	}
	return out
}

// requestBodiesAdmittingExtraKeys walks every operation and content type, JSON
// or not, and answers each open body with the operations that take it.
func requestBodiesAdmittingExtraKeys(t *testing.T) map[string][]string {
	t.Helper()
	doc := loadContractDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("the contract has no paths")
	}
	found := map[string][]string{}
	for path, itemAny := range paths {
		item, _ := itemAny.(map[string]any)
		for method, opAny := range item {
			op, _ := opAny.(map[string]any)
			body, _ := op["requestBody"].(map[string]any)
			content, _ := body["content"].(map[string]any)
			for _, mediaAny := range content {
				media, _ := mediaAny.(map[string]any)
				schema, _ := media["schema"].(map[string]any)
				operation := strings.ToUpper(method) + " " + path
				for _, name := range openBodies(doc, schema, "(inline body of "+operation+")", map[string]bool{}) {
					found[name] = append(found[name], operation)
				}
			}
		}
	}
	return found
}

func TestEveryBodyThatTakesExtraKeysIsListedHereAndTakesCustomFields(t *testing.T) {
	t.Parallel()
	found := requestBodiesAdmittingExtraKeys(t)
	if len(found) == 0 {
		t.Fatal("no operation takes a body that admits extra keys; the walk no longer sees the contract")
	}
	for name, operations := range found {
		if freeFormBodies.Waived(t, name) {
			continue
		}
		if _, listed := customFieldRequestTypes[name]; !listed {
			t.Errorf("%s admits extra keys (%s) and is not in customFieldRequestTypes. Only a body that takes custom fields may; close any other with additionalProperties: false", name, strings.Join(operations, ", "))
		}
	}
	freeFormBodies.AssertAllMatched(t)
	for name := range customFieldRequestTypes {
		if _, used := found[name]; !used {
			t.Errorf("%s is listed and no operation takes it as a body that admits extra keys", name)
		}
		object := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(name, "Create"), "Update"), "Renew"), "Request"))
		if !fieldcatalog.Target(object).Valid() {
			t.Errorf("%s admits extra keys for %q, which is no custom-field target: a cf_ key there would be accepted and never stored", name, object)
		}
	}
}

// refusedOnTheWire decodes a body through the REST decode and answers the
// status and the problem body.
//
//craft:ignore naked-any the decode target is whichever generated request struct the case names
func refusedOnTheWire(body string, into any) (int, string) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/records", strings.NewReader(body))
	if httperr.Decode(rec, req, into) {
		return http.StatusOK, ""
	}
	return rec.Code, rec.Body.String()
}

func TestEveryCustomFieldBodyRefusesAMisspeltKeyOnBothTransports(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a misspelt key":            `{"misspelt_field":1}`,
		"a case variant":            `{"FULL_NAME":"x"}`,
		"a bare cf_ prefix":         `{"cf_":1}`,
		"a misspelt key beside cf_": `{"cf_tier":"gold","misspelt_field":1}`,
	}
	for name, typ := range customFieldRequestTypes {
		for label, body := range cases {
			t.Run(name+" "+label, func(t *testing.T) {
				status, problem := refusedOnTheWire(body, reflect.New(typ).Interface())
				if status != http.StatusUnprocessableEntity || !strings.Contains(problem, "unknown_field") {
					t.Fatalf("the web wire answered %d %s, want 422 unknown_field", status, problem)
				}
				err := datasource.StrictDecode(json.RawMessage(body), reflect.New(typ).Interface())
				if _, ok := errors.AsType[*datasource.FieldDecodeError](err); !ok {
					t.Fatalf("the tool seam answered %v, want a FieldDecodeError", err)
				}
			})
		}
		t.Run(name+" takes a cf_ key", func(t *testing.T) {
			if status, problem := refusedOnTheWire(`{"cf_tier":"gold"}`, reflect.New(typ).Interface()); status != http.StatusOK {
				t.Fatalf("the web wire refused a cf_ key: %d %s", status, problem)
			}
			if err := datasource.StrictDecode(json.RawMessage(`{"cf_tier":"gold"}`), reflect.New(typ).Interface()); err != nil {
				t.Fatalf("the tool seam refused a cf_ key: %v", err)
			}
		})
	}
}

// A misspelt key inside a nested row vanished while the top level was held.
func TestAMisspeltKeyInsideANestedRowIsRefused(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"CreateContactRequest", "UpdateContactRequest"} {
		typ := customFieldRequestTypes[name]
		body := `{"emails":[{"emial":"a@b.example"}]}`
		status, problem := refusedOnTheWire(body, reflect.New(typ).Interface())
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s on the web wire answered %d %s, want 422", name, status, problem)
		}
		if err := datasource.StrictDecode(json.RawMessage(body), reflect.New(typ).Interface()); err == nil {
			t.Errorf("%s on the tool seam accepted a misspelt key inside an email row", name)
		}
	}
}
