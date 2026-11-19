// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// wireFault is one entry of a 422's details.errors, as a client reads it.
type wireFault struct {
	Field   string            `json:"field"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Params  map[string]string `json:"params"`
}

// refusedFault runs body through Decode into target and answers the one field
// fault the 422 names.
//
//craft:ignore naked-any the decode target is whichever request struct a case names
func refusedFault(t *testing.T, body string, target any) wireFault {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/things", strings.NewReader(body))
	if Decode(rec, req, target) {
		t.Fatalf("body %s was accepted", body)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("body %s answered %d %s, want 422", body, rec.Code, rec.Body)
	}
	var problem struct {
		Details struct {
			Errors []wireFault `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding problem body %q: %v", rec.Body.String(), err)
	}
	if len(problem.Details.Errors) != 1 {
		t.Fatalf("body %s named %d faults, want 1: %s", body, len(problem.Details.Errors), rec.Body)
	}
	return problem.Details.Errors[0]
}

// A value of the wrong shape names the key the client sent. Its code is chosen by
// the shape the field takes, so a screen can say what to enter.
func TestDecodeNamesTheFieldAndShapeOfAValueItCannotHold(t *testing.T) {
	type typed struct {
		Email   *openapi_types.Email `json:"email,omitempty"`
		At      *time.Time           `json:"at,omitempty"`
		Day     *openapi_types.Date  `json:"day,omitempty"`
		OwnerID *openapi_types.UUID  `json:"owner_id,omitempty"`
		Active  *bool                `json:"active,omitempty"`
	}
	cases := map[string]struct {
		body   string
		target any
		field  string
		code   string
	}{
		"a fraction for a whole number": {`{"win_probability":33.5}`, &crmcontracts.UpdateDealRequest{}, "win_probability", "not_integer"},
		"text for a whole number":       {`{"win_probability":"ten"}`, &crmcontracts.UpdateDealRequest{}, "win_probability", "not_integer"},
		"text for a number":             {`{"name":"W","unit_price_minor":0,"default_tax_rate":"x"}`, &crmcontracts.CreateProductRequest{}, "default_tax_rate", "not_number"},
		"no date for a date":            {`{"expected_close_date":"31/12/2026"}`, &crmcontracts.UpdateDealRequest{}, "expected_close_date", "invalid_date"},
		"no id for an id":               {`{"owner_id":"me"}`, &typed{}, "owner_id", "invalid_uuid"},
		"no address for an email":       {`{"email":"not-an-address"}`, &typed{}, "email", "email_malformed"},
		"a number for an email":         {`{"email":5}`, &typed{}, "email", "email_malformed"},
		"no timestamp for a date-time":  {`{"at":"yesterday"}`, &typed{}, "at", "datetime_malformed"},
		"text for a boolean":            {`{"active":"yes"}`, &typed{}, "active", "invalid_type"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fault := refusedFault(t, tc.body, tc.target)
			if fault.Field != tc.field || fault.Code != tc.code {
				t.Errorf("named %s/%s, want %s/%s", fault.Field, fault.Code, tc.field, tc.code)
			}
			if fault.Message == "" {
				t.Error("the fault carries no English message for an API caller")
			}
		})
	}
}

// A body no field can be blamed for keeps the code that says so, on the body.
func TestDecodeKeepsMalformedJSONForABodyThatIsNotOne(t *testing.T) {
	for _, body := range []string{`{"win_probability":`, `{"name":"a"} {}`, `[1]`} {
		fault := refusedFault(t, body, &crmcontracts.UpdateDealRequest{})
		if fault.Field != "body" || fault.Code != "malformed_json" {
			t.Errorf("%s named %s/%s, want body/malformed_json", body, fault.Field, fault.Code)
		}
	}
}

// A range refusal states its bounds as the contract spells them, so a screen
// can show them in the unit its reader typed.
func TestDecodeStatesTheBoundsOfARangeRefusal(t *testing.T) {
	cases := map[string]struct {
		body   string
		target any
		field  string
		params map[string]string
	}{
		"both bounds":      {`{"win_probability":150}`, &crmcontracts.UpdateDealRequest{}, "win_probability", map[string]string{"min": "0", "max": "100"}},
		"a lower bound":    {`{"name":"W","unit_price_minor":-500}`, &crmcontracts.CreateProductRequest{}, "unit_price_minor", map[string]string{"min": "0"}},
		"a date-time span": {`{"expected_close_date":"9999-12-31"}`, &crmcontracts.UpdateDealRequest{}, "expected_close_date", map[string]string{"min": "0001-01-02", "max": "9999-12-30"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fault := refusedFault(t, tc.body, tc.target)
			if fault.Field != tc.field || fault.Code != "out_of_range" || !maps.Equal(fault.Params, tc.params) {
				t.Errorf("named %s/%s %v, want %s/out_of_range %v", fault.Field, fault.Code, fault.Params, tc.field, tc.params)
			}
		})
	}
}

// A value decoded under a named argument is named by its path inside it.
func TestUnmarshalOrRefusalNamesTheKeyUnderItsArgument(t *testing.T) {
	var fields crmcontracts.UpdateDealRequest
	err := UnmarshalOrRefusal(t.Context(), "fields", json.RawMessage(`{"win_probability":"x"}`), &fields)
	refused, ok := errors.AsType[*DetailedError](err)
	if !ok || len(refused.Fields) != 1 {
		t.Fatalf("err = %v, want one field refusal", err)
	}
	if got := refused.Fields[0]; got.Field != "fields.win_probability" || got.Code != "not_integer" {
		t.Errorf("named %s/%s, want fields.win_probability/not_integer", got.Field, got.Code)
	}
}
