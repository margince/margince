// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// A company text field the contract bounds and the column the database guard
// bounds must name one limit. Wider in the contract, a request the schema calls
// valid fails in the database. Narrower, a client cuts text the record could
// hold. The fields come from the company write bodies and the limits from
// columns.txt. A new field or a changed limit is then checked with no edit here.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

// companyWriteBodies are the request bodies whose fields land on company
// columns of the same name.
var companyWriteBodies = []string{"CreateCompanyRequest", "UpdateCompanyRequest"}

// serverWrittenCompanyColumns are the bounded company text columns no request
// body writes, each with the writer that fills it instead.
// gatekit:fixture the bounded company columns only the server writes, each with its writer
var serverWrittenCompanyColumns = map[string]string{
	"captured_by":          "the signed-in caller, never a request body",
	"geocode_input_hash":   "the geocoder",
	"geocode_source":       "the geocoder",
	"geocode_status":       "the geocoder",
	"logo_object_key":      "a logo upload or a site read, which carry a file",
	"logo_source":          "a logo upload or a site read, which carry a file",
	"logo_icon_object_key": "a logo upload or a site read, which carry a file",
	"logo_icon_source":     "a logo upload or a site read, which carry a file",
	"name_source":          "the writer that named the company",
}

func TestEveryCompanyTextFieldPublishesItsColumnsLimit(t *testing.T) {
	t.Parallel()
	raw := headCatalog(t)
	columns, err := textbounds.Columns(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	policies, err := textbounds.Policies()
	if err != nil {
		t.Fatal(err)
	}
	companyText := map[string]textbounds.Column{}
	for _, column := range columns {
		if column.Table == "company" {
			companyText[column.Name] = column
		}
	}
	contract := loadContractDocument(t)
	reached := map[string]bool{}
	for _, body := range companyWriteBodies {
		fields := companyFieldSchemas(t, contract, body)
		for name, field := range fields {
			published, bounded := field["maxLength"].(int)
			column, isText := companyText[name]
			if !isText {
				if bounded {
					t.Errorf("%s publishes maxLength %d for %s, and company has no text column of that name", body, published, name)
				}
				continue
			}
			reached[name] = true
			// An enum is held by the column's CHECK, and an exempt column has no
			// guard limit to agree with; its writers check the contract's bound.
			limit := policies[column.Key].Limit
			if field["enum"] != nil || limit == 0 {
				continue
			}
			switch {
			case !bounded:
				t.Errorf("%s.%s publishes no maxLength, and the database refuses past %d: add maxLength: %d to api/crm.yaml", body, name, limit, limit)
			case published != limit:
				t.Errorf("%s.%s publishes maxLength %d, and columns.txt bounds %s at %d: change one so both say the same", body, name, published, column.Key, limit)
			}
		}
	}
	// A walk that misses a field checks less and still passes. So every bounded
	// column is reached by a body or named as one the server writes.
	for name, column := range companyText {
		writer, serverWritten := serverWrittenCompanyColumns[name]
		switch {
		case policies[column.Key].Limit == 0:
		case reached[name] && serverWritten:
			t.Errorf("%s is named as written by %s, and a company write body sends it: drop it from serverWrittenCompanyColumns", column.Key, writer)
		case !reached[name] && !serverWritten:
			t.Errorf("no company write body reaches %s: publish it in api/crm.yaml or name its writer in serverWrittenCompanyColumns", column.Key)
		}
	}
	for name := range serverWrittenCompanyColumns {
		if _, exists := companyText[name]; !exists {
			t.Errorf("serverWrittenCompanyColumns names %s, which is no company text column", name)
		}
	}
}

// companyFieldSchemas reads a body's properties by the company column each one
// writes. A property that refers to an object schema, such as the address,
// writes one column per nested property, named `<property>_<nested>`.
func companyFieldSchemas(t *testing.T, contract map[string]any, body string) map[string]map[string]any {
	t.Helper()
	properties := schemaProperties(contract, body)
	if len(properties) == 0 {
		t.Fatalf("api/crm.yaml has no properties for %s: the gate's subject moved; repoint it", body)
	}
	fields := map[string]map[string]any{}
	for name, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ref, isRef := property["$ref"].(string)
		if !isRef {
			fields[name] = property
			continue
		}
		nested := schemaProperties(contract, strings.TrimPrefix(ref, "#/components/schemas/"))
		for nestedName, nestedRaw := range nested {
			if nestedProperty, ok := nestedRaw.(map[string]any); ok {
				fields[name+"_"+nestedName] = nestedProperty
			}
		}
	}
	return fields
}
