// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheFollowUpWindowIsOneToThirtyDays(t *testing.T) {
	t.Parallel()
	for days, admit := range map[int]bool{0: false, 1: true, 2: true, 30: true, 31: false} {
		raw, err := json.Marshal(days)
		if err != nil {
			t.Fatal(err)
		}
		err = FollowUpAfterDays.ValidateJSON(raw)
		if admit && err != nil {
			t.Errorf("%d days refused: %v", days, err)
		}
		if !admit && err == nil {
			t.Errorf("%d days admitted, want refused", days)
		}
	}
}

// Unwired, both endpoints refuse rather than answer a default nobody can
// change; a body that is not the schema is refused before anything is read.
func TestTheFollowUpEndpointsRefuseWhenTheSettingsAreNotWired(t *testing.T) {
	t.Parallel()
	h := Handlers{}
	get := httptest.NewRecorder()
	h.GetFollowUpSettings(get, httptest.NewRequest(http.MethodGet, "/v1/activities/follow-up-settings", nil))
	if get.Code < 500 {
		t.Errorf("GET without settings = %d, want a server error", get.Code)
	}
	patch := httptest.NewRecorder()
	h.UpdateFollowUpSettings(patch, httptest.NewRequest(http.MethodPatch, "/v1/activities/follow-up-settings",
		strings.NewReader(`{"follow_up_after_days": 3}`)))
	if patch.Code < 500 {
		t.Errorf("PATCH without settings = %d, want a server error", patch.Code)
	}
	garbled := httptest.NewRecorder()
	h.UpdateFollowUpSettings(garbled, httptest.NewRequest(http.MethodPatch, "/v1/activities/follow-up-settings",
		strings.NewReader(`not json`)))
	if garbled.Code != http.StatusUnprocessableEntity {
		t.Errorf("PATCH with a garbled body = %d, want 422", garbled.Code)
	}
}
