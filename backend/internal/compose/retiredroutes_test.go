// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestARetiredRouteAnswersANamedRefusal(t *testing.T) {
	recorder := httptest.NewRecorder()
	Server{}.GetForecastAssuranceEnforcement(recorder, httptest.NewRequest(http.MethodGet, "/v1/forecast/assurance/enforcement", nil))
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("a retired route answered %d", recorder.Code)
	}
}

func TestARetiredForwardMeasureSavesAsTheMeasureItWasBuiltOn(t *testing.T) {
	for sent, want := range map[string]string{"qualified_likely": "likely", "qualified_weighted": "weighted", "manager_call": "manager_call", "qualified_manager_call": "qualified_manager_call"} {
		measure := crmcontracts.UpdateInstallationSettingsRequestForecastForwardMeasure(sent)
		if got := savedForwardMeasure(&measure); got == nil || *got != want {
			t.Errorf("%s saved as %v, want %s", sent, got, want)
		}
	}
	if savedForwardMeasure[string](nil) != nil {
		t.Error("an absent measure became a saved one")
	}
}
