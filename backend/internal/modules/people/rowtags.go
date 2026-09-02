// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// wireRowTags renders one row's tag chips for the person and organization
// lists, which both draw them. The deals module carries its own twin, because
// a module never imports a sibling. Both render the contract's generated
// RowTag, so a change to the shape is a compile error in each.
func wireRowTags(tags []storekit.RowTag) *[]crmcontracts.RowTag {
	out := make([]crmcontracts.RowTag, 0, len(tags))
	for _, t := range tags {
		var color *crmcontracts.RowTagColor
		if t.Color != nil {
			c := crmcontracts.RowTagColor(*t.Color)
			color = &c
		}
		out = append(out, crmcontracts.RowTag{
			TagId: openapi_types.UUID(t.TagID), Name: t.Name, Color: color,
		})
	}
	return &out
}
