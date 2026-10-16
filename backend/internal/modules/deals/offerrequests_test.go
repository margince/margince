// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// refusedOn answers the field and code a request rule refused on, whichever
// of the two refusal shapes the rule speaks.
func refusedOn(t *testing.T, err error) (field, code string) {
	t.Helper()
	var detailed *httperr.DetailedError
	if errors.As(err, &detailed) && len(detailed.Fields) == 1 {
		return detailed.Fields[0].Field, detailed.Fields[0].Code
	}
	return faultOf(t, err)
}

func TestAnOfferCreateRefusesEveryTotalTheServerDerives(t *testing.T) {
	for name, body := range map[string]string{
		"a header total": `{"currency":"EUR","source":"manual","total_minor":100}`,
		"a line total": `{"currency":"EUR","source":"manual",
			"line_items":[{"quantity":1,"unit_price_minor":100,"line_net_minor":100}]}`,
	} {
		_, err := offerCreateInput([]byte(body))
		if field, code := refusedOn(t, err); code != "totals_derived" {
			t.Errorf("%s was refused on %s/%s, want totals_derived", name, field, code)
		}
	}
}

func TestAnOfferCreateNamesWhatIsWrongWithItsBody(t *testing.T) {
	for name, tc := range map[string]struct{ body, field, code string }{
		"malformed json":    {`{"currency":`, "body", "malformed_json"},
		"line items as one": {`{"currency":"EUR","source":"manual","line_items":{}}`, "body", "malformed_json"},
		"a blank source":    {`{"currency":"EUR","source":"  "}`, "source", "required"},
	} {
		_, err := offerCreateInput([]byte(tc.body))
		if field, code := refusedOn(t, err); field != tc.field || code != tc.code {
			t.Errorf("%s was refused on %s/%s, want %s/%s", name, field, code, tc.field, tc.code)
		}
	}
}

func TestAnOfferCreateMapsItsDateAndLinesOneWay(t *testing.T) {
	in, err := offerCreateInput([]byte(`{"currency":"EUR","source":" manual ","valid_until":"2099-12-31",
		"line_items":[{"description":"Setup","quantity":2,"unit_price_minor":5000,"tax_rate":19}]}`))
	if err != nil {
		t.Fatalf("a clean create body was refused: %v", err)
	}
	if in.Currency != "EUR" || in.Source != "manual" {
		t.Errorf("currency %q and source %q, want EUR and the trimmed manual", in.Currency, in.Source)
	}
	if in.ValidUntil == nil || *in.ValidUntil != "2099-12-31" {
		t.Errorf("valid_until mapped to %v, want the ISO day", in.ValidUntil)
	}
	if len(in.LineItems) != 1 || in.LineItems[0].Quantity != "2" || *in.LineItems[0].TaxRate != "19" {
		t.Errorf("the line items mapped to %+v, want one line of quantity 2 at 19%% tax", in.LineItems)
	}
}

func TestAnOfferPatchRefusesATotalAndCarriesItsClears(t *testing.T) {
	var smuggled crmcontracts.UpdateOfferRequest
	if err := json.Unmarshal([]byte(`{"gross_minor":1}`), &smuggled); err != nil {
		t.Fatal(err)
	}
	_, err := offerUpdateInput(smuggled, nil, nil)
	if field, code := refusedOn(t, err); field != "gross_minor" || code != "totals_derived" {
		t.Fatalf("a patch naming gross_minor was refused on %s/%s, want gross_minor/totals_derived", field, code)
	}

	var req crmcontracts.UpdateOfferRequest
	if err := json.Unmarshal([]byte(`{"valid_until":"2099-06-30","intro_text":"Hello"}`), &req); err != nil {
		t.Fatal(err)
	}
	version := int64(4)
	in, err := offerUpdateInput(req, []string{"terms_text"}, &version)
	if err != nil {
		t.Fatalf("a clean patch was refused: %v", err)
	}
	if in.ValidUntil == nil || *in.ValidUntil != "2099-06-30" || *in.IntroText != "Hello" {
		t.Errorf("the patch mapped to %+v", in)
	}
	if !slices.Equal(in.Clear, []string{"terms_text"}) || in.IfVersion != &version {
		t.Errorf("clears %v and version %v did not travel with the patch", in.Clear, in.IfVersion)
	}
}

func TestALineItemRefusesATotalTheServerDerives(t *testing.T) {
	add := func(raw []byte) error { _, err := addLineInput(raw); return err }
	patch := func(raw []byte) error { _, err := updateLineInput(raw); return err }
	for name, tc := range map[string]struct {
		mapLine func([]byte) error
		body    string
		field   string
	}{
		"an added line": {add, `{"quantity":1,"line_total_minor":100}`, "line_total_minor"},
		"a line patch":  {patch, `{"tax_minor":7}`, "tax_minor"},
	} {
		if field, code := refusedOn(t, tc.mapLine([]byte(tc.body))); field != tc.field || code != "totals_derived" {
			t.Errorf("%s was refused on %s/%s, want %s/totals_derived", name, field, code, tc.field)
		}
	}
}

func TestALineItemRefusesAFieldTheContractDoesNotName(t *testing.T) {
	var decode *datasource.FieldDecodeError
	if _, err := addLineInput([]byte(`{"quantity":1,"unit_prize_minor":100}`)); !errors.As(err, &decode) {
		t.Errorf("an added line with a misspelt price answered %v, want a decode refusal", err)
	}
	if _, err := updateLineInput([]byte(`{"qty":2}`)); !errors.As(err, &decode) {
		t.Errorf("a line patch with an unknown qty key answered %v, want a decode refusal", err)
	}
}

func TestALinePatchMapsEveryFigureItNames(t *testing.T) {
	in, err := updateLineInput([]byte(`{"quantity":3,"discount_pct":10,"tax_rate":7,
		"billing_model":"recurring","billing_interval_months":12,"interval_count":2}`))
	if err != nil {
		t.Fatalf("a clean line patch was refused: %v", err)
	}
	if *in.Quantity != "3" || *in.DiscountPct != "10" || *in.TaxRate != "7" {
		t.Errorf("figures mapped to %v/%v/%v, want 3/10/7", *in.Quantity, *in.DiscountPct, *in.TaxRate)
	}
	if !in.Classified || *in.BillingModel != "recurring" || *in.BillingIntervalMonths != 12 {
		t.Errorf("the classification mapped to %+v, want recurring every 12 months", in)
	}
	if !in.IntervalCountSet || *in.IntervalCount != 2 {
		t.Errorf("interval_count mapped to %v, want it set to 2", in.IntervalCount)
	}
}

func TestALinePatchNamingNoClassificationLeavesItAlone(t *testing.T) {
	in, err := updateLineInput([]byte(`{"description":"Training"}`))
	if err != nil {
		t.Fatalf("a description patch was refused: %v", err)
	}
	if in.Classified || in.IntervalCountSet || in.Quantity != nil {
		t.Errorf("a description patch touched more than the description: %+v", in)
	}
}
