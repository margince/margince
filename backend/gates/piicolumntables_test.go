// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "sort"

// redactedTables are the registered PII tables the cascade redacts, the ones
// the Art. 17 column census judges. A table the cascade only deletes from
// loses the whole row, so it has no column set to account for.
func redactedTables(redacted map[string]map[string]bool) []string {
	out := make([]string, 0, len(redacted))
	for table := range redacted {
		if _, registered := piiTables[table]; registered {
			out = append(out, table)
		}
	}
	sort.Strings(out)
	return out
}
