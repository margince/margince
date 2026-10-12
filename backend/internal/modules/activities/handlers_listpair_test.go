// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheTimelineRefusesHalfAnEntityPairInsteadOfListingEverything(t *testing.T) {
	entityType := crmcontracts.ListActivitiesParamsEntityType("contact")
	entityID := crmcontracts.Id{}
	for name, params := range map[string]crmcontracts.ListActivitiesParams{
		"id alone":   {EntityId: &entityID},
		"type alone": {EntityType: &entityType},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			// The refusal precedes any store read, so no store is needed.
			Handlers{}.ListActivities(rec, httptest.NewRequest(http.MethodGet, "/v1/activities", nil), params)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s answered %d, want 422", name, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "requires_pair") {
				t.Fatalf("%s refusal does not say the pair is incomplete: %s", name, rec.Body.String())
			}
		})
	}
}
