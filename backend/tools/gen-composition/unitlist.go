// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"path/filepath"
	"sort"
)

// unitListFile names the composed units and where each one's manifest is,
// relative to the root. scripts/verify-boot.sh reads it instead of guessing
// the set from the folders under extensions/.
const unitListFile = "units.json"

// composedUnit is one row of unitListFile.
type composedUnit struct {
	Name     string `json:"name"`
	Manifest string `json:"manifest"`
}

// composedUnitList encodes the selected units sorted by name. A core-only build
// gets an empty array, so a reader can tell "no units" from a missing file.
func composedUnitList(root string, units []extensionUnit) ([]byte, error) {
	rows := make([]composedUnit, 0, len(units))
	for _, u := range units {
		rel, err := filepath.Rel(root, filepath.Join(u.Dir, unitManifestFile))
		if err != nil {
			return nil, err
		}
		rows = append(rows, composedUnit{Name: u.Name, Manifest: filepath.ToSlash(rel)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	encoded, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
