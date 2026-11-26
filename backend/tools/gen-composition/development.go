// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "github.com/margince/margince/backend/internal/platform/unitset"

const developmentOnlyMarker = unitset.DevelopmentOnlyMarker

// selectedInstanceUnits is the set this run composes: see unitset.Select.
func selectedInstanceUnits(root string) ([]unitset.Unit, error) {
	units, err := instanceUnits(root)
	if err != nil {
		return nil, err
	}
	return unitset.Select(units, *includeDevelopment), nil
}
