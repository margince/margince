// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"regexp"
	"strings"
)

var nonCatalogKeyRun = regexp.MustCompile(`[^a-z0-9]+`)

// CatalogKey turns a settings-list label such as "Trade show" into the key the
// list stores behind it, "trade_show". The key outlives a rename of the label,
// so every administered list derives it the same way.
func CatalogKey(label string) string {
	key := nonCatalogKeyRun.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "_")
	return strings.Trim(key, "_")
}
