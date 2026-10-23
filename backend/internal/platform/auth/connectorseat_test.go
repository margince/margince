// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"strings"
	"testing"
)

// Readers splice these fragments into fmt format strings, where a '%' would
// turn the query into a syntax error at read time.
func TestConnectorProvenanceFragmentsSurviveAFormatString(t *testing.T) {
	t.Parallel()
	for name, fragment := range map[string]string{
		"seat":   ConnectorSeatSQL("a"),
		"stands": ParticipantStandsSQL("ap", "a"),
	} {
		if strings.Contains(fragment, "%") {
			t.Errorf("the %s fragment carries a %%: %s", name, fragment)
		}
	}
}
