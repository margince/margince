// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// geocodePointsDataTask moves the points resolved before they lived beside the
// company onto the side row. Until it reaches a company, radius answers leave
// that company out: an omission, never a distance from a previous address.
func geocodePointsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793138103_geocode_points_beside_the_company", Version: 1,
		Title: "Move company geocode points beside the company", Requires: []string{"core/1793138100"},
		Steps: []datatask.Step{
			foldStep("copy_company_points", "contacts writes the same point on the side row the geocode worker writes, without a separate audit",
				contacts.CopyCompanyPointsTx, contacts.CountUncopiedCompanyPoints),
		},
	}
}
