// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// A record write names the fields the record has, or a cf_ key. The schemas
// that admit custom fields set additionalProperties, so the generated decoder
// takes any key. Only the key check both transports share stands between a
// misspelt field and a write that stores nothing. This gate derives the bodies
// from the contract and proves each one is held.

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// extensionRequestTypes maps each request schema that admits custom fields to
// its generated type. The census below reads the contract, so a schema added
// there and missing here fails.
var extensionRequestTypes = map[string]reflect.Type{
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
	"CreateOfferRequest":    reflect.TypeFor[crmcontracts.CreateOfferRequest](),
	"UpdateOfferRequest":    reflect.TypeFor[crmcontracts.UpdateOfferRequest](),
	"CreateProductRequest":  reflect.TypeFor[crmcontracts.CreateProductRequest](),
	"UpdateProductRequest":  reflect.TypeFor[crmcontracts.UpdateProductRequest](),
	"CreateProjectRequest":  reflect.TypeFor[crmcontracts.CreateProjectRequest](),
	"UpdateProjectRequest":  reflect.TypeFor[crmcontracts.UpdateProjectRequest](),
}

func extensionRequestSchemas(t *testing.T) []string {
	t.Helper()
	doc := loadContractDocument(t)
	schemas, ok := doc["components"].(map[string]any)["schemas"].(map[string]any)
	if !ok {
		t.Fatal("the contract has no components.schemas")
	}
	var names []string
	for name, node := range schemas {
		schema, ok := node.(map[string]any)
		if ok && schema["additionalProperties"] == true && strings.HasSuffix(name, "Request") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func TestEveryCustomFieldRequestBodyIsListedHere(t *testing.T) {
	census := extensionRequestSchemas(t)
	if len(census) == 0 {
		t.Fatal("the contract names no request schema that admits custom fields; the census is short")
	}
	for _, name := range census {
		if _, listed := extensionRequestTypes[name]; !listed {
			t.Errorf("%s admits custom fields and is not in extensionRequestTypes, so nothing proves a misspelt key is refused", name)
		}
	}
	for name := range extensionRequestTypes {
		if !slices.Contains(census, name) {
			t.Errorf("%s is listed and the contract no longer lists it as admitting custom fields", name)
		}
	}
}

func TestEveryCustomFieldRequestBodyRefusesAMisspeltKey(t *testing.T) {
	for name, typ := range extensionRequestTypes {
		t.Run(name, func(t *testing.T) {
			body := reflect.New(typ).Interface()
			err := datasource.RejectNonCanonicalKeys(json.RawMessage(`{"misspelt_field":1}`), body)
			if _, ok := errors.AsType[*datasource.UnknownFieldError](err); !ok {
				t.Fatalf("a misspelt key was answered %v, want an UnknownFieldError", err)
			}
			if err := datasource.RejectNonCanonicalKeys(json.RawMessage(`{"cf_custom_field":1}`), body); err != nil {
				t.Fatalf("a cf_ key was refused: %v", err)
			}
		})
	}
}

// A body that admits extra keys must be listed here. The census above reads
// schema names, and this one reads the operations, so a catch-all schema a
// route takes under another name is found too.
func TestEveryRequestBodyThatAdmitsExtraKeysIsListedHere(t *testing.T) {
	doc := loadContractDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("the contract has no paths")
	}
	seen := 0
	for path, itemAny := range paths {
		item, _ := itemAny.(map[string]any)
		for method, opAny := range item {
			op, _ := opAny.(map[string]any)
			body, _ := op["requestBody"].(map[string]any)
			content, _ := body["content"].(map[string]any)
			media, _ := content["application/json"].(map[string]any)
			schema, _ := media["schema"].(map[string]any)
			ref, _ := schema["$ref"].(string)
			resolved, found := resolveSchemaRef(doc, ref)
			if !found || resolved["additionalProperties"] != true {
				continue
			}
			seen++
			name := ref[strings.LastIndex(ref, "/")+1:]
			if _, listed := extensionRequestTypes[name]; !listed {
				t.Errorf("%s %s takes %s, which admits extra keys and is not in extensionRequestTypes", strings.ToUpper(method), path, name)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no operation takes a body that admits extra keys; the walk no longer sees the contract")
	}
}
