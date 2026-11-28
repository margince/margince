// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func tagColorError(color *string) error {
	if color == nil || crmcontracts.RecordTagColor(*color).Valid() {
		return nil
	}
	return &BadInputError{Field: "color", Reason: "choose a color from the tag palette"}
}
