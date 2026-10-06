// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "strings"

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
