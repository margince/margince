// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package workarea answers which area of work an agent's call lands in, so a
// passport limited to some areas can be held to them. The table is generated
// by the gate that places every operation for docs/reference/mcp-parity.md
// (backend/gates/mcpparity_test.go), so the page and the running server read
// one placement.
package workarea

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed workareas.json
var tableJSON []byte

// File is the generated table's shape.
type File struct {
	Note  string `json:"note"`
	Areas []Area `json:"areas"`
	// Operations places each operation an agent can call, by operationId.
	Operations     map[string]string   `json:"operations"`
	Placements     map[string]string   `json:"placements"`
	VerbPlacements map[string][]string `json:"verb_placements"`
	// Verbs lists the areas of the operations each verb serves.
	Verbs map[string][]string `json:"verbs"`
	// RecordCalls places a verb that picks its operation by record type.
	RecordCalls map[string]map[string]string `json:"record_calls"`
}

// Area is one area of work. Only a choosable area is offered to a user. A free
// area is admitted on every passport, and one HeldAs another is on when that
// one is.
type Area struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Choosable bool   `json:"choosable"`
	Free      bool   `json:"free,omitempty"`
	HeldAs    string `json:"held_as,omitempty"`
}

var table = mustRead(tableJSON)

func mustRead(raw []byte) File {
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		//craft:ignore panic-in-domain the table is embedded at build time, and TestTheEmbeddedTableReads reads it in every unit run
		panic(fmt.Sprintf("workarea: workareas.json does not read: %v", err))
	}
	return file
}

// Embedded is the table this binary carries, for the gates that hold it to
// the contract and the registry.
func Embedded() File { return table }

// Choosable lists the areas a user may turn on or off, in the table's order.
func Choosable() []Area {
	var out []Area
	for _, area := range table.Areas {
		if area.Choosable {
			out = append(out, area)
		}
	}
	return out
}

// IsChoosable reports whether key names an area a user may choose.
func IsChoosable(key string) bool {
	return slices.ContainsFunc(table.Areas, func(area Area) bool { return area.Key == key && area.Choosable })
}

// SwitchOf is the choosable area whose switch admits a call placed in area. free
// reports an area every passport is admitted in; key is then empty.
func SwitchOf(area string) (key string, free bool) {
	for _, known := range table.Areas {
		if known.Key != area {
			continue
		}
		switch {
		case known.Free:
			return "", true
		case known.HeldAs != "":
			return known.HeldAs, false
		default:
			return known.Key, false
		}
	}
	return area, false
}

// NameOf is the name a user knows an area by, or key itself for an unknown one.
func NameOf(key string) string {
	for _, area := range table.Areas {
		if area.Key == key {
			return area.Name
		}
	}
	return key
}

// OfOperation is the area of one operation, false when the table places none.
func OfOperation(operationID string) (string, bool) {
	area, ok := table.Operations[operationID]
	return area, ok
}

// OfVerb lists the areas of every operation a verb serves. A verb that serves
// none, such as an extension's tool, is in no area.
func OfVerb(verb string) []string {
	return slices.Clone(table.Verbs[verb])
}

// OfRecordCall is the area of a call whose record type picks the operation.
func OfRecordCall(verb, recordType string) (string, bool) {
	area, ok := table.RecordCalls[verb][recordType]
	return area, ok
}

// PlacementOfOperation reads the parity catalog's area for an HTTP operation.
func PlacementOfOperation(operationID string) (string, bool) {
	placement, ok := table.Placements[operationID]
	return placement, ok
}

// PlacementsOfVerb returns a copy so callers cannot change the shared catalog.
func PlacementsOfVerb(verb string) []string {
	return slices.Clone(table.VerbPlacements[verb])
}
