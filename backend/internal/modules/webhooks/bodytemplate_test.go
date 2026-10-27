// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testEnvelope = `{
  "event_id": "0190a7f2-0000-7000-8000-000000000001",
  "type": "deal.stage_changed",
  "version": 1,
  "occurred_at": "2026-10-14T09:12:00Z",
  "actor": {"type": "user"},
  "entity": {"type": "deal", "id": "0190a7f2-0000-7000-8000-000000000002"},
  "correlation_id": "0190a7f2-0000-7000-8000-000000000003",
  "data": {
    "name": "Acme \"renewal\" <2027>",
    "amount_minor": 1234567890123456,
    "deal_amount_minor": 5683020,
    "currency": "USD",
    "yen": "JPY",
    "lowercase_currency": "usd",
    "won": false,
    "tags": ["a", "b"],
    "nested": {"k": "v"},
    "empty": null
  }
}`

func render(t *testing.T, src, contentType string) string {
	t.Helper()
	tpl, err := parseBodyTemplate(src, contentType)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := tpl.render([]byte(testEnvelope))
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return string(out)
}

func parseFails(t *testing.T, src, contentType, wantReason string) {
	t.Helper()
	_, err := parseBodyTemplate(src, contentType)
	bad, ok := errors.AsType[*BadInputError](err)
	if !ok {
		t.Fatalf("parse %q: want a BadInputError, got %v", src, err)
	}
	if bad.Field != fieldBodyTemplate {
		t.Errorf("field = %q, want %q", bad.Field, fieldBodyTemplate)
	}
	if !strings.Contains(bad.Reason, wantReason) {
		t.Errorf("reason = %q, want it to contain %q", bad.Reason, wantReason)
	}
}

func TestJSONTemplateEscapesAStringValueInsideAJSONString(t *testing.T) {
	got := render(t, `{"text": "Deal {{ data.name }} moved"}`, contentTypeJSON)
	want := `{"text": "Deal Acme \"renewal\" <2027> moved"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if !json.Valid([]byte(got)) {
		t.Fatal("rendered body is not valid JSON")
	}
}

func TestJSONFilterEmitsTheOriginalBytesSoLargeIntegersKeepTheirPrecision(t *testing.T) {
	got := render(t, `{"amount": {{ data.amount_minor | json }}, "tags": {{data.tags|json}}}`, contentTypeJSON)
	want := `{"amount": 1234567890123456, "tags": ["a","b"]}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestANonStringValueInsideAJSONStringIsItsJSONText(t *testing.T) {
	got := render(t, `{"s": "{{ data.amount_minor }} {{ data.won }} {{ data.nested }} {{ version }}"}`, contentTypeJSON)
	want := `{"s": "1234567890123456 false {\"k\":\"v\"} 1"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestAMissingOrNullPathIsEmptyInAStringAndNullAsJSON(t *testing.T) {
	got := render(t, `{"a": "{{ data.absent }}", "b": {{ data.absent | json }}, "c": "{{ data.empty }}", "d": {{ data.tags.5 | json }}}`, contentTypeJSON)
	want := `{"a": "", "b": null, "c": "", "d": null}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestAnArrayIndexSegmentResolves(t *testing.T) {
	if got := render(t, `{"first": "{{ data.tags.1 }}", "who": "{{ actor.type }}"}`, contentTypeJSON); got != `{"first": "b", "who": "user"}` {
		t.Fatalf("got %s", got)
	}
}

func TestTextTemplateEmitsValuesAsTheyAre(t *testing.T) {
	got := render(t, "Deal {{ data.name }} at {{ occurred_at }}: {{ data.tags | json }}", contentTypeText)
	want := `Deal Acme "renewal" <2027> at 2026-10-14T09:12:00Z: ["a","b"]`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestFormTemplateQueryEscapesEveryValue(t *testing.T) {
	got := render(t, "name={{ data.name }}&tags={{ data.tags | json }}", contentTypeForm)
	want := "name=Acme+%22renewal%22+%3C2027%3E&tags=%5B%22a%22%2C%22b%22%5D"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestTheContentTypeDefaultsToJSON(t *testing.T) {
	if got := render(t, `{"t": "{{ type }}"}`, ""); got != `{"t": "deal.stage_changed"}` {
		t.Fatalf("got %s", got)
	}
}

func TestATemplateTheEngineCannotRenderSafelyIsRefusedAtSave(t *testing.T) {
	for _, tc := range []struct{ name, src, contentType, reason string }{
		{"placeholder outside a JSON string without json", `{"n": {{ data.amount_minor }}}`, contentTypeJSON, "line 1, column 7: a placeholder outside a JSON string needs | json"},
		{"json filter inside a JSON string", `{"n": "{{ data.tags | json }}"}`, contentTypeJSON, "inside a JSON string"},
		{"unknown root", "{\n  \"x\": \"{{ deal.name }}\"\n}", contentTypeJSON, `line 2, column 9: "deal" is not a field of the event`},
		{"empty placeholder", `{"x": "{{ }}"}`, contentTypeJSON, "names no field"},
		{"empty segment", `{"x": "{{ data..name }}"}`, contentTypeJSON, "names no field"},
		{"unknown filter", `{"x": {{ data.name | upper }}}`, contentTypeJSON, `"upper" is not a filter`},
		{"no filter after the bar", `{"x": "{{ data.name | }}"}`, contentTypeJSON, "names no filter"},
		{"money without a currency field", `{"x": "{{ data.amount_minor | money }}"}`, contentTypeJSON, "money needs the field that holds the currency"},
		{"money with a currency outside the event", `{"x": "{{ data.amount_minor | money deal.currency }}"}`, contentTypeJSON, `"deal" is not a field of the event`},
		{"money outside a JSON string", `{"x": {{ data.amount_minor | money data.currency }}}`, contentTypeJSON, "needs | json, or put it inside quotes"},
		{"unclosed placeholder", `{"x": "{{ data.name"}`, contentTypeJSON, "is never closed"},
		{"invalid JSON around the placeholders", `{"x": "{{ data.name }}",}`, contentTypeJSON, "is not valid JSON"},
		{"escape running into a placeholder", `{"x": "\{{ data.name }}"}`, contentTypeJSON, "backslash"},
		{"unicode escape split by a placeholder", `{"a": "\u00{{ type }}41"}`, contentTypeJSON, "\\u escape"},
		{"unicode escape cut at its start", `{"a": "\u{{ type }}0041"}`, contentTypeJSON, "\\u escape"},
		{"empty template", "  ", contentTypeText, "is empty"},
		{"too large", strings.Repeat("x", maxTemplateBytes+1), contentTypeText, "16 KiB"},
		{"too many placeholders", strings.Repeat("{{ type }}", maxPlaceholders+1), contentTypeText, "the limit is 500"},
	} {
		t.Run(tc.name, func(t *testing.T) { parseFails(t, tc.src, tc.contentType, tc.reason) })
	}
}

func TestMoneyRendersMinorUnitsInTheCurrencysOwnDigits(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"two-digit currency", `{{ data.deal_amount_minor | money data.currency }}`, "56,830.20 USD"},
		{"zero-digit currency", `{{ data.deal_amount_minor | money data.yen }}`, "5,683,020 JPY"},
		{"a figure too large for a float", `{{ data.amount_minor | money data.currency }}`, "12,345,678,901,234.56 USD"},
		{"no amount", `{{ data.missing | money data.currency }}`, ""},
		{"no currency", `{{ data.deal_amount_minor | money data.missing }}`, ""},
		{"not an ISO code", `{{ data.deal_amount_minor | money data.lowercase_currency }}`, ""},
		{"not a whole number", `{{ data.name | money data.currency }}`, ""},
	} {
		if got := render(t, tc.src, contentTypeText); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMoneyIsEscapedLikeAnyTextInJSONAndForms(t *testing.T) {
	if got := render(t, `{"a": "{{ data.deal_amount_minor | money data.currency }}"}`, contentTypeJSON); got != `{"a": "56,830.20 USD"}` {
		t.Errorf("json: %s", got)
	}
	if got := render(t, `a={{ data.deal_amount_minor | money data.currency }}`, contentTypeForm); got != "a=56%2C830.20+USD" {
		t.Errorf("form: %s", got)
	}
}

func TestGroupThousandsKeepsTheSignInFront(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "999": "999", "1000": "1,000", "-1234567": "-1,234,567", "-100": "-100"} {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAContentTypeOutsideTheAllowListIsRefused(t *testing.T) {
	_, err := parseBodyTemplate(`x`, "application/xml")
	if bad, ok := errors.AsType[*BadInputError](err); !ok || bad.Field != fieldBodyContentType {
		t.Fatalf("err = %v, want a BadInputError on %s", err, fieldBodyContentType)
	}
}

func TestARenderedBodyOverTheCapFails(t *testing.T) {
	tpl, err := parseBodyTemplate(strings.Repeat("{{ data.name }}", maxPlaceholders), contentTypeText)
	if err != nil {
		t.Fatal(err)
	}
	big := `{"data": {"name": "` + strings.Repeat("y", 600) + `"}}`
	if _, err := tpl.render([]byte(big)); !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("err = %v, want errBodyTooLarge", err)
	}
}
