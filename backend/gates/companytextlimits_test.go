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
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// companyWriteBodies are the request bodies whose fields land on company
// columns of the same name.
var companyWriteBodies = []string{"CreateCompanyRequest", "UpdateCompanyRequest"}

func TestEveryCompanyTextFieldPublishesItsColumnsLimit(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := textbounds.Columns(strings.NewReader(string(raw)))
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
	checked := map[string]bool{}
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
			// An enum is held by the column's CHECK, and an exempt column has no
			// guard limit to agree with; its writers check the contract's bound.
			limit := policies[column.Key].Limit
			if field["enum"] != nil || limit == 0 {
				continue
			}
			checked[name] = true
			switch {
			case !bounded:
				t.Errorf("%s.%s publishes no maxLength, and the database refuses past %d: add maxLength: %d to api/crm.yaml", body, name, limit, limit)
			case published != limit:
				t.Errorf("%s.%s publishes maxLength %d, and columns.txt bounds %s at %d: change one so both say the same", body, name, published, column.Key, limit)
			}
		}
	}
	// The address arrives as one nested object. A walk that stopped resolving it
	// would check fewer columns and still pass, so each bounded address column
	// must have been reached.
	for name, column := range companyText {
		if strings.HasPrefix(name, "address_") && policies[column.Key].Limit != 0 && !checked[name] {
			t.Errorf("no company write body reached %s: the walk over nested objects no longer matches the contract", column.Key)
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
