// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

const developmentOnlyMarker = "margince:development-only"

// Selection precedes fetching and every generated artifact, so excluded examples
// contribute no code, contracts, screens or migrations to a production build.
func selectedInstanceUnits(root string) ([]unitSource, error) {
	units, err := instanceUnits(root)
	if err != nil {
		return nil, err
	}
	selected := units[:0]
	for _, unit := range units {
		if !unit.developmentOnly || *includeDevelopment {
			selected = append(selected, unit)
		}
	}
	return selected, nil
}
