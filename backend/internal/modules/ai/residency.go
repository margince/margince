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
	"europe-west12":     true,
	"europe-north1":     true,
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

// residencyBound reports whether p limits where a model may process a
// prompt: on the same host (sovereign), or inside the EU (eu_resident).
func (p Profile) residencyBound() bool {
	return p == ProfileSovereign || p == ProfileEUResident
}

// RequireResidency refuses a binding the profile does not let process a
// prompt, and is asked before any client is built for it. eu_resident admits
// what sovereign admits, and gemini_vertex at a resident location.
func RequireResidency(profile Profile, binding ProviderConfig) error {
	return refuseNonResident(profile, "this binding", binding)
}

func refuseNonResident(profile Profile, label string, binding ProviderConfig) error {
	if !profile.residencyBound() {
		return nil
	}
	if profile == ProfileEUResident && binding.Provider == providerGeminiVertex {
		return requireEULocation(label, binding.Location)
	}
	if !localProviders[binding.Provider] {
		if profile == ProfileEUResident {
			return fmt.Errorf("ai: routing config: profile %s forbids cloud provider %q on %s: only gemini_vertex at an EU location, or a same-host model, keeps processing inside the EU",
				profile, binding.Provider, label)
		}
		return fmt.Errorf("ai: routing config: profile %s forbids cloud provider %q on %s", profile, binding.Provider, label)
	}
	return requireSovereignEndpoint(label, binding.Provider, binding.BaseURL)
}

func requireEULocation(label, location string) error {
	if err := vertexLocationError(label, location); err != nil {
		return err
	}
	if euResidentLocations[location] {
		return nil
	}
	reason, named := nonResidentReason[location]
	if !named {
		reason = "the EU locations are " + strings.Join(slices.Sorted(maps.Keys(euResidentLocations)), ", ")
	}
	return fmt.Errorf("ai: routing config: profile %s refuses gemini_vertex at location %q on %s: %s",
		ProfileEUResident, location, label, reason)
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
