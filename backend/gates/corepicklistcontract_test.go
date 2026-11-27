// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The core picklist value sets against the contract that owns them.
//
// A filter builder offers a picklist's values instead of asking a reader to type
// one, and the sets it offers are declared beside the engine (collections'
// vocab.go) while the contract enumerates them in api/crm.yaml. Drift between the
// two is silent and runs both ways: a value the contract dropped stays on offer
// and selects nothing, and a value it gained is one no builder shows — which is
// the free-text failure the whole surface exists to remove, arriving one enum
// member at a time.
//
// Here rather than in the module, for the reason the root's other contract gates
// carry: this walks the authoritative 3.1 document rather than the generated Go,
// so a contract-only edit cannot slip past it, and collections does not take a
// YAML dependency to hold a claim about a document it does not own.
//
// Reading the document also lets the null be dropped deliberately. oapi-codegen
// emits a `<nil>` member for a nullable enum (CompanySizeBandLessThannil is
// real), so a set derived from the generated constants would carry a value no
// human should be offered; a null in the contract is the COLUMN's nullability,
// and `exists: false` is how a filter asks for empty.
//
// The COMPLETENESS half — that every core picklist offers values at all, that a
// retired one offers none, and that no offered value is empty or `<nil>` — is
// collections' own TestEveryCorePicklistOffersItsValues. This holds the sets it
// finds to the document, which is why an empty set is skipped here rather than
// asserted twice.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// contractDocument is the OpenAPI document these sets mirror.
const contractDocument = "api/crm.yaml"

// Where each offered set lives in the contract, keyed `<resource>.<field>`.
//
// The property is named separately from the field because the two can differ: an
// account's relationship is `relationship_types` on the schema (an account may
// hold several) and `relationship_type` as a filter leaf, which selects accounts
// that hold AT LEAST the named one.
//
// A picklist with no entry here FAILS rather than being skipped, so a core
// picklist added tomorrow is covered the day it lands — by forcing whoever adds
// it to say where the contract admits its values.
var picklistInContract = map[string]struct{ schema, property string }{
	"company.status":            {"Company", "status"},
	"company.lifecycle":         {"Company", "status"},
	"company.industry":          {"Company", "industry"},
	"company.size_band":         {"Company", "size_band"},
	"company.relationship_type": {"Company", "relationship_types"},
	"deal.status":               {"Deal", "status"},
	"lead.status":               {"Lead", "status"},
	"project.phase":             {"Project", "phase"},
	// A deal filters on its customer's account through a link leaf, so two of the
	// sets above appear a second time under the deal engine. Both entries name the
	// Company property they reach, which is the point: a link leaf offering a
	// different set from the field it reads is exactly the drift this catches.
	"deal.company_status":    {"Company", "status"},
	"deal.company_lifecycle": {"Company", "status"},
	"deal.company_size_band": {"Company", "size_band"},
	"deal.company_industry":  {"Company", "industry"},
	"lead.company_industry":  {"Company", "industry"},
	"lead.industry":          {"Company", "industry"},
	// The technical leaves read `company_fact.value_key`, which is a bare
	// string on the fact schema and cannot carry three different enums. Each
	// names the dedicated schema that publishes ITS set instead — which is what
	// makes a client able to offer the values at all.
	"company.mail_provider":    {"TechnicalMailProvider", ""},
	"company.hosting_provider": {"TechnicalHostingProvider", ""},
	"company.operated_service": {"TechnicalOperatedService", ""},
}

func TestEveryOfferedPicklistMatchesTheContractsValues(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	compared := 0
	for _, resource := range vocabularyResources(t, doc) {
		engine, ok, err := (&collections.Store{}).SegmentEngine(context.Background(), resource)
		if err != nil {
			t.Fatalf("segment engine for %s: %v", resource, err)
		}
		if !ok {
			t.Errorf("the contract admits resource %q and no engine serves it", resource)
			continue
		}
		for name, field := range engine.Fields {
			if field.Type != storekit.FieldPicklist || len(field.Options) == 0 {
				continue
			}
			where, declared := picklistInContract[resource+"."+name]
			if !declared {
				t.Errorf("%s.%s offers values and nothing says where the contract admits them, so nothing holds the two together", resource, name)
				continue
			}
			compared++
			compareValueSets(t, resource+"."+name, field.Options, contractEnum(t, doc, where.schema, where.property))
		}
	}
	if compared == 0 {
		t.Fatal("no picklist was compared, so this gate checked nothing")
	}
}

// compareValueSets reports each direction separately, because the two failures
// read nothing alike to whoever has to fix them.
func compareValueSets(t *testing.T, field string, offered []string, admitted map[string]bool) {
	t.Helper()
	offers := map[string]bool{}
	for _, value := range offered {
		offers[value] = true
		if !admitted[value] {
			t.Errorf("%s offers %q and the contract does not admit it, so picking it composes a clause that selects nothing", field, value)
		}
	}
	for value := range admitted {
		if !offers[value] {
			t.Errorf("%s: the contract admits %q and the field does not offer it, so a builder cannot compose a clause the engine accepts", field, value)
		}
	}
	if len(admitted) == 0 {
		t.Errorf("%s: the contract enum read as empty, so this compared nothing", field)
	}
}

// vocabularyResources reads the resources GET /filters/vocabulary admits, so a
// sixth one is swept by existing rather than by being added to a list here.
func vocabularyResources(t *testing.T, doc map[string]any) []string {
	t.Helper()
	operation := descendContract(t, doc, "paths", "/filters/vocabulary", "get")
	params, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatal("GET /filters/vocabulary declares no parameters, so the resource set is unreadable")
	}
	for _, raw := range params {
		param, isMap := raw.(map[string]any)
		if !isMap || param["name"] != "resource" {
			continue
		}
		schema, isMap := param["schema"].(map[string]any)
		if !isMap {
			t.Fatal("the resource parameter carries no schema")
		}
		if ref, isRef := schema["$ref"].(string); isRef {
			schema = descendContract(t, doc, "components", "schemas", strings.TrimPrefix(ref, "#/components/schemas/"))
		}
		return enumStrings(t, schema, "the resource parameter")
	}
	t.Fatal("GET /filters/vocabulary declares no resource parameter")
	return nil
}

// contractEnum answers one property's enum, minus null.
//
// An array-typed property (relationship_types) keeps its enum on `items`, so both
// spellings are read: the admitted values are the same set either way, and which
// one a schema uses is not this gate's business.
func contractEnum(t *testing.T, doc map[string]any, schema, property string) map[string]bool {
	t.Helper()
	// An empty property names a schema that IS the enum — the shape a set needs
	// when the column publishing it is a bare string that several sets share.
	if property == "" {
		node := descendContract(t, doc, "components", "schemas", schema)
		admitted := map[string]bool{}
		for _, value := range enumStrings(t, node, schema) {
			admitted[value] = true
		}
		return admitted
	}
	node := descendContract(t, doc, "components", "schemas", schema, "properties", property)
	if _, direct := node["enum"]; !direct {
		items, isMap := node["items"].(map[string]any)
		if !isMap {
			t.Fatalf("%s.%s has neither an enum nor items to read one from", schema, property)
		}
		node = items
	}
	admitted := map[string]bool{}
	for _, value := range enumStrings(t, node, schema+"."+property) {
		admitted[value] = true
	}
	return admitted
}

// enumStrings reads a schema node's enum as strings. A null member is dropped —
// it is the column's nullability, not a value to offer — and anything else
// non-string is named rather than skipped, since it would be a contract shape
// this gate cannot read.
func enumStrings(t *testing.T, node map[string]any, subject string) []string {
	t.Helper()
	raw, ok := node["enum"].([]any)
	if !ok {
		// A set a unit may extend spells its core values as x-extensible-enum.
		raw, ok = node["x-extensible-enum"].([]any)
	}
	if !ok {
		t.Fatalf("%s carries no enum", subject)
	}
	var values []string
	for _, member := range raw {
		text, isText := member.(string)
		if !isText {
			if member != nil {
				t.Errorf("%s enum carries a non-string member %#v", subject, member)
			}
			continue
		}
		values = append(values, text)
	}
	return values
}

func descendContract(t testing.TB, doc map[string]any, path ...string) map[string]any {
	t.Helper()
	node := doc
	for _, key := range path {
		next, ok := node[key].(map[string]any)
		if !ok {
			t.Fatalf("the contract has no %s under %v", key, path)
		}
		node = next
	}
	return node
}

// loadContractDocument answers the caller's own copy of the contract, decoded
// once per test binary: a reader may change the tree it walks.
func loadContractDocument(t *testing.T) map[string]any {
	t.Helper()
	decodedContract.Lock()
	defer decodedContract.Unlock()
	if decodedContract.doc == nil {
		decodedContract.doc = loadOpenAPIDocument(t, contractDocument)
	}
	return copyYAMLMapping(decodedContract.doc)
}

// decodedContract stays empty when a decode fails, so the next caller fails
// the same way.
var decodedContract struct {
	sync.Mutex
	doc map[string]any
}

// copyYAMLMapping copies every mapping and sequence of a decoded YAML mapping.
// The scalars it holds cannot change.
func copyYAMLMapping(mapping map[string]any) map[string]any {
	out := make(map[string]any, len(mapping))
	for key, item := range mapping {
		out[key] = copyYAMLValue(item)
	}
	return out
}

//craft:ignore naked-any a decoded YAML value is any by the decoder's own contract
func copyYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyYAMLMapping(typed)
	case map[any]any:
		out := make(map[any]any, len(typed))
		for key, item := range typed {
			out[key] = copyYAMLValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyYAMLValue(item)
		}
		return out
	default:
		return value
	}
}

// loadOpenAPIDocument reads one OpenAPI document as untyped nodes, for the
// readers above. The path is a parameter because the product publishes two —
// crm.yaml to callers and public-events.yaml to subscribers — and a set spelled
// in both is read here twice rather than by two readers.
func loadOpenAPIDocument(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc map[string]any
	if err := unmarshalYAML(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return doc
}

// A caller's change to its contract document leaves the next caller's as it was.
func TestACallersChangeToTheContractDocumentStaysItsOwn(t *testing.T) {
	t.Parallel()
	required := func(doc map[string]any) []any {
		list, ok := descendContract(t, doc, "components", "schemas", "RbacObjectGrant")["required"].([]any)
		if !ok || len(list) == 0 {
			t.Fatalf("%s's RbacObjectGrant has no required list to change", contractDocument)
		}
		return list
	}
	first := loadContractDocument(t)
	want := fmt.Sprint(required(first))
	required(first)[0] = "changed"
	delete(descendContract(t, first, "components"), "schemas")
	if got := fmt.Sprint(required(loadContractDocument(t))); got != want {
		t.Errorf("a caller's change reached the next caller: required %s, want %s", got, want)
	}
}
