// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testdb

import "regexp"

var indexedIDComparison = regexp.MustCompile(`(?:^|\()(?:[a-z_][a-z_0-9]*\.)?id\s*=\s*`)

// IndexConditionReadsID recognizes id equality without pinning one physical index name.
func IndexConditionReadsID(condition string) bool {
	return indexedIDComparison.MatchString(condition)
}
