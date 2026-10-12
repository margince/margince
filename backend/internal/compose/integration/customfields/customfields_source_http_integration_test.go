// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

// The contract requires `source` on a custom field, as on every create that
// records an origin. The harness adds one to a create that lacks it, so these
// requests go out verbatim.

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestACustomFieldCreateWithoutASourceIsRefused(t *testing.T) {
	e := schemaWiredEnv(t)
	for label, stated := range map[string]integration.AnyMap{
		"absent": {}, "empty": {"source": ""}, "spaces": {"source": "   "},
	} {
		t.Run(label, func(t *testing.T) {
			body := integration.AnyMap{"object": "deal", "label": "Origin " + label, "type": "text"}
			maps.Copy(body, stated)
			var problem customFieldProblem
			if status := e.CallVerbatim(t, "POST", "/v1/custom-fields", body, nil, &problem); status != http.StatusUnprocessableEntity {
				t.Fatalf("POST /v1/custom-fields %v → %d, want 422", body, status)
			}
			if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "source" {
				t.Errorf("refusal names %+v, want the field source", problem.Details.Errors)
			}
		})
	}
	t.Run("stated", func(t *testing.T) {
		body := integration.AnyMap{"object": "deal", "label": "Origin stated", "type": "text", "source": "manual"}
		if status := e.CallVerbatim(t, "POST", "/v1/custom-fields", body, nil, nil); status != http.StatusCreated {
			t.Errorf("POST /v1/custom-fields with a source → %d, want 201", status)
		}
	})
}
