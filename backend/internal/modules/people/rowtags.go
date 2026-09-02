// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// wireRowTags renders one row's tag chips. Written once here because the
// person and organization lists both draw them and the shape is the contract's,
// not either list's.
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
