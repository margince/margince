// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func inquiryPolicyFixture() map[string]any {
	return map[string]any{
		"form_id":        "00000000-0000-7000-8000-000000000001",
		"coordinator_id": "00000000-0000-7000-8000-000000000002",
		"mode":           "one_colleague", "routing": map[string]any{"owners": []any{"00000000-0000-7000-8000-000000000003"}},
		"task_title": "Answer the website inquiry", "due_in_days": 1, "timezone": "Europe/Berlin",
	}
}

func TestInquiryPoliciesRequireACompleteTypedConfiguration(t *testing.T) {
	cases := []struct {
		name, field string
		change      func(map[string]any)
	}{
		{"complete", "", func(map[string]any) {}},
		{"coordinator review", "", func(p map[string]any) { p["mode"] = "coordinator"; p["routing"] = map[string]any{} }},
		{"rotation", "", func(p map[string]any) { p["mode"] = "rotate" }},
		{"missing routing", "params.routing", func(p map[string]any) { delete(p, "routing") }},
		{"null routing", "params.routing", func(p map[string]any) { p["routing"] = nil }},
		{"missing form", "params.form_id", func(p map[string]any) { delete(p, "form_id") }},
		{"missing coordinator", "params.coordinator_id", func(p map[string]any) { delete(p, "coordinator_id") }},
		{"unknown mode", "params.mode", func(p map[string]any) { p["mode"] = "team_queue" }},
		{"capacity needs a separate reviewed feature", "params.routing", func(p map[string]any) {
			p["routing"] = map[string]any{"owners": []any{"00000000-0000-7000-8000-000000000003"}, "cap_per_owner": 1}
		}},
		{"territory needs a separate reviewed feature", "params.routing", func(p map[string]any) {
			p["routing"] = map[string]any{"owners": []any{"00000000-0000-7000-8000-000000000003"}, "rules": []any{map[string]any{"field": "origin", "equals": "website", "owner_id": "00000000-0000-7000-8000-000000000003"}}}
		}},
		{"unknown knob", "params", func(p map[string]any) { p["send_email"] = true }},
		{"host timezone", "params.timezone", func(p map[string]any) { p["timezone"] = "Local" }},
		{"padded timezone", "params.timezone", func(p map[string]any) { p["timezone"] = " Europe/Berlin " }},
		{"invisible title", "params.task_title", func(p map[string]any) { p["task_title"] = "\u200b" }},
		{"empty title", "params.task_title", func(p map[string]any) { p["task_title"] = " " }},
		{"long title", "params.task_title", func(p map[string]any) { p["task_title"] = strings.Repeat("界", 201) }},
		{"unicode title", "", func(p map[string]any) { p["task_title"] = strings.Repeat("界", 200) }},
		{"zero offset", "params.due_in_days", func(p map[string]any) { p["due_in_days"] = 0 }},
		{"coordinator cannot assign ownership", "params.routing", func(p map[string]any) { p["mode"] = "coordinator" }},
		{"one colleague cannot have an empty pool", "params.routing.owners", func(p map[string]any) { p["routing"] = map[string]any{} }},
		{"empty rotation", "params.routing.owners", func(p map[string]any) { p["mode"] = "rotate"; p["routing"] = map[string]any{} }},
		{"unknown routing knob", "params", func(p map[string]any) { p["routing"] = map[string]any{"custom_queue": true} }},
		{"duplicate colleagues", "params.routing.owners", func(p map[string]any) {
			p["mode"] = "rotate"
			p["routing"] = map[string]any{"owners": []any{"00000000-0000-7000-8000-000000000003", "00000000-0000-7000-8000-000000000003"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := inquiryPolicyFixture()
			tc.change(p)
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseInquiryPolicy(raw)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			field, ok := errors.AsType[*ParamError](err)
			if !ok || field.Field != tc.field {
				t.Fatalf("refusal = %v, want %s", err, tc.field)
			}
		})
	}
}
