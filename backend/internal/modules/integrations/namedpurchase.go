// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// namedPurchase answers the provider a run started by hand names, trimmed, and
// refuses one that names none or sends an empty category list. An omitted list
// stays the connection's own selection.
func namedPurchase(in provider.QueueInput) (string, error) {
	name := strings.TrimSpace(in.Provider)
	if name == "" {
		return "", &UnnamedPurchaseError{Field: "provider", Reason: "name the provider to buy from"}
	}
	if in.Categories != nil && len(in.Categories) == 0 {
		return "", &UnnamedPurchaseError{
			Field:  "categories",
			Reason: "name at least one category, or leave categories out for the connection's own selection",
		}
	}
	return name, nil
}
