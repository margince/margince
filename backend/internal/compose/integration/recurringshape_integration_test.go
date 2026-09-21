// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The billing-classification shape CHECKs, exercised against a live schema.
//
// These are asserted at the DATABASE rather than through the store, because
// what is being proved is that the constraint refuses a row — not that a
// validator in front of it does. A Go check can be bypassed by a tool, a
// restore or a future writer; the constraint is what holds for all of them.
//
// The case that makes this file worth having is "a cadence with no model".
// Written the obvious way — `billing_model = 'one_time' AND ...` — every arm
// of the shape disjunction evaluates to NULL for an unclassified row, and a
// CHECK that evaluates NULL PASSES. The first version of this migration
// admitted that row. It was caught by running exactly these inserts against
// the migrated schema, which is the only way that class of mistake is visible:
// the constraint looks right, the catalog diff looks right, and nothing fails.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// shapeCase is one row and the constraint that must refuse it, or "" where the
// row is one of the legal shapes.
type shapeCase struct {
	name       string
	columns    string
	values     string
	constraint string
}

func TestTheProductBillingShapeAdmitsOnlyThreeClassifications(t *testing.T) {
	e := Setup(t)

	const base = "name, unit_price_minor, currency, source, captured_by"
	const baseValues = "'Probe', 1000, 'EUR', 'manual', 'test'"

	for _, c := range []shapeCase{
		// The three legal shapes. Unclassified is first because it is what
		// every row written before this column existed carries, and defaulting
		// it to one_time would assert a classification nobody made.
		{name: "unclassified, as every legacy row is", columns: "", values: ""},
		{name: "one_time with no cadence", columns: ", billing_model", values: ", 'one_time'"},
		{
			name:    "recurring with a cadence",
			columns: ", billing_model, billing_interval_months",
			values:  ", 'recurring', 3",
		},

		// The refusals.
		{
			name: "recurring with no cadence", columns: ", billing_model",
			values: ", 'recurring'", constraint: "product_billing_shape",
		},
		{
			name: "one_time carrying a cadence", columns: ", billing_model, billing_interval_months",
			values: ", 'one_time', 12", constraint: "product_billing_shape",
		},
		{
			// The null-evaluation case. A cadence is a claim about a recurring
			// price, so a row carrying one while claiming no model at all is
			// half a classification.
			name: "a cadence with no model at all", columns: ", billing_interval_months",
			values: ", 12", constraint: "product_billing_shape",
		},
		{
			name: "a cadence nobody bills on", columns: ", billing_model, billing_interval_months",
			values: ", 'recurring', 5", constraint: "product_billing_interval_check",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := e.WsExecErr(t,
				"INSERT INTO product ("+base+c.columns+") VALUES ("+baseValues+c.values+")")
			assertShape(t, err, c.constraint)
		})
	}
}

func TestTheOfferLineBillingShapeAdmitsOnlyThreeClassifications(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Recurring shape probe", pipeline, open, &e.Rep1)
	e.WsExec(t,
		`INSERT INTO offer (deal_id, offer_number, currency, source, captured_by)
		 VALUES ($1, 'OF-SHAPE-1', 'EUR', 'manual', 'test')`, deal)

	const base = "offer_id, position, description, quantity, unit_price_minor"

	for i, c := range []shapeCase{
		{name: "unclassified, as every legacy line is", columns: "", values: ""},
		{name: "one_time with neither", columns: ", billing_model", values: ", 'one_time'"},
		{
			// The count is optional while drafting: a line can be classified
			// before anybody has settled how long the commitment runs. Send
			// refuses one that still has none, which is a lifecycle rule and
			// lives in Go where it can say so.
			name:    "recurring with a cadence and no count yet",
			columns: ", billing_model, billing_interval_months",
			values:  ", 'recurring', 12",
		},
		{
			name:    "recurring with a cadence and a count",
			columns: ", billing_model, billing_interval_months, interval_count",
			values:  ", 'recurring', 3, 4",
		},

		{
			name: "recurring with no cadence", columns: ", billing_model",
			values: ", 'recurring'", constraint: "oli_billing_shape",
		},
		{
			name: "a cadence with no model at all", columns: ", billing_interval_months",
			values: ", 12", constraint: "oli_billing_shape",
		},
		{
			name: "a count with no model at all", columns: ", interval_count",
			values: ", 4", constraint: "oli_billing_shape",
		},
		{
			// Periods of what? A one-off price repeats zero times by
			// definition, so a count on it is a commitment to nothing.
			name: "one_time carrying a count", columns: ", billing_model, interval_count",
			values: ", 'one_time', 4", constraint: "oli_billing_shape",
		},
		{
			name:    "a commitment to zero periods",
			columns: ", billing_model, billing_interval_months, interval_count",
			values:  ", 'recurring', 3, 0", constraint: "oli_interval_count_check",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A distinct position per case: the position is the line's order on
			// the paper and a collision would fail for a reason this test is
			// not about.
			err := e.WsExecErr(t,
				"INSERT INTO offer_line_item ("+base+c.columns+")"+
					" SELECT o.id, "+strconv.Itoa(i+1)+", 'Probe', 1, 1000"+c.values+
					" FROM offer o LIMIT 1")
			assertShape(t, err, c.constraint)
		})
	}
}

// assertShape checks the row was refused by the named constraint, or accepted
// where no constraint is named. A refusal by the WRONG constraint is reported
// as its own failure: it would mean the case is proving something other than
// what it says.
func assertShape(t *testing.T, err error, constraint string) {
	t.Helper()
	if constraint == "" {
		if err != nil {
			t.Fatalf("a legal shape was refused: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("the row was accepted, but %s exists to refuse it", constraint)
	}
	if !strings.Contains(err.Error(), constraint) {
		t.Fatalf("refused by something other than %s: %v", constraint, err)
	}
}

// A product can be un-classified, which needs a word of its own on the wire.
//
// The generated request field is a pointer, so a JSON null and an omitted field
// both arrive as nil — and an omitted field has to keep meaning "leave the
// classification alone", or every unrelated product edit would wipe it. Without
// `not_specified` the form's "Not specified" option is a control that silently
// does nothing, and a product once classified could never go back to saying
// nothing about whether its price repeats.
func TestAProductCanGoBackToSayingNothingAboutRepeating(t *testing.T) {
	e := Setup(t)
	// The catalogue is administered under its own object grant, which the
	// harness's default admin does not carry.
	admin := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"deal_desk"},
		Objects: map[string]principal.ObjectGrant{
			"product": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})

	months := 3
	created, err := e.Deals.CreateProduct(admin, deals.CreateProductInput{
		Name: "Quarterly support", UnitPriceMinor: 300_000, Currency: "EUR", Source: "manual",
		BillingModel: strPtr(deals.BillingRecurring), BillingIntervalMonths: &months,
	})
	if err != nil {
		t.Fatalf("create a classified product: %v", err)
	}
	id := ids.From[ids.ProductKind](ids.UUID(created.Id))

	// An unrelated edit leaves the classification exactly where it was. This is
	// the behaviour the flag protects, and the reason a bare null cannot mean
	// "clear".
	renamed := "Quarterly support, renamed"
	after, err := e.Deals.UpdateProduct(admin, id, deals.UpdateProductInput{Name: &renamed})
	if err != nil {
		t.Fatalf("an unrelated edit: %v", err)
	}
	if after.BillingModel == nil || string(*after.BillingModel) != deals.BillingRecurring {
		t.Fatal("an unrelated edit dropped the classification")
	}

	// And addressing it explicitly clears both halves.
	cleared, err := e.Deals.UpdateProduct(admin, id, deals.UpdateProductInput{Classified: true})
	if err != nil {
		t.Fatalf("un-classifying the product: %v", err)
	}
	if cleared.BillingModel != nil || cleared.BillingIntervalMonths != nil {
		t.Fatalf("the product still says model=%v interval=%v, want both absent",
			cleared.BillingModel, cleared.BillingIntervalMonths)
	}
}
