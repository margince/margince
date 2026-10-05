// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// euResidentLocations are the Vertex AI locations whose ML processing Google
// keeps inside EU member states: the EU multi-region and the EU regions.
// London and Zürich are European and outside the EU, so they are not here.
var euResidentLocations = map[string]bool{
	"eu":                true,
	"europe-west1":      true,
	"europe-west3":      true,
	"europe-west4":      true,
	"europe-west8":      true,
	"europe-west9":      true,
	"europe-west10":     true,
	"europe-west12":     true,
	"europe-north1":     true,
	"europe-north2":     true,
	"europe-central2":   true,
	"europe-southwest1": true,
}

// nonResidentReason names why a location an operator might expect to be
// resident is not.
var nonResidentReason = map[string]string{
	"europe-west2": "London is outside the EU",
	"europe-west6": "Zürich is outside the EU",
	vertexGlobal:   "the global endpoint may process the prompt anywhere",
	"us":           "the US multi-region processes in the US",
}

// vertexLocationGap names why a gemini_vertex binding may process a prompt
// outside the EU, or answers "" when its location is an EU one. Any other
// provider answers "": its host is not a location this rule can read.
func vertexLocationGap(binding ProviderConfig) string {
	if binding.Provider != providerGeminiVertex || euResidentLocations[binding.Location] {
		return ""
	}
	reason, named := nonResidentReason[binding.Location]
	if !named {
		reason = "the EU locations are " + strings.Join(slices.Sorted(maps.Keys(euResidentLocations)), ", ")
	}
	return fmt.Sprintf("location %q is not an EU location: %s", binding.Location, reason)
}

// The jurisdictions a Vertex location is reported under.
const (
	jurisdictionEU     = "eu"
	jurisdictionUS     = "us"
	jurisdictionOther  = "other"
	jurisdictionGlobal = "global"
)

// locationJurisdiction is whose law a Vertex location processes under, by
// this build's policy: eu exactly when resident, so a location Google adds
// reads as other until this build names it.
func locationJurisdiction(location string) string {
	switch {
	case euResidentLocations[location]:
		return jurisdictionEU
	case location == vertexGlobal:
		return jurisdictionGlobal
	case location == "us" || strings.HasPrefix(location, "us-"):
		return jurisdictionUS
	default:
		return jurisdictionOther
	}
}
