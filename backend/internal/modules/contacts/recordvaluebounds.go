// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func recordVisibilityError(visibility *string) error {
	if visibility == nil || crmcontracts.ContactVisibility(*visibility).Valid() {
		return nil
	}
	return httperr.Validation("visibility", "invalid_value", "choose owner or workspace visibility")
}
