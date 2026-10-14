// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type importRunMappingDTO struct {
	Version int `json:"version"`
	Parts   []struct {
		Mapping map[string]string `json:"mapping"`
	} `json:"parts"`
}

func readRunMapping(t *testing.T, e *apptest.AppEnv, id string) importRunMappingDTO {
	t.Helper()
	var typed importRunMappingDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+id, nil, nil, &typed); status != http.StatusOK {
		t.Fatalf("GET /v1/imports/%s → %d, want 200", id, status)
	}
	return typed
}

func TestARunReturnsTheMappingThatWasSavedOnIt(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	want := map[string]string{"Email": "email", "Full Name": "full_name"}
	if status := e.Call(t, http.MethodPatch, "/v1/imports/"+profile.RunID, AnyMap{
		"version": 1, "object": "lead", "mapping": want,
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH mapping → %d, want 200", status)
	}

	got := readRunMapping(t, e, profile.RunID)
	if len(got.Parts) != 1 || !maps.Equal(got.Parts[0].Mapping, want) {
		t.Fatalf("run parts = %+v, want the saved mapping %v returned as sent", got.Parts, want)
	}
}

func TestAnUploadedRunReturnsTheMappingItWasStagedWith(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)

	got := readRunMapping(t, e, profile.RunID)
	if len(got.Parts) != 1 || !maps.Equal(got.Parts[0].Mapping, profile.SuggestedMapping) {
		t.Fatalf("run parts = %+v, want the suggestion %v it was staged with", got.Parts, profile.SuggestedMapping)
	}
}
