// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

func TestTheSuggestedSourceNameIsOneTheStageAccepts(t *testing.T) {
	for _, file := range []string{"HubSpot export (2).zip", "pipedrive-export.zip", "Ümlaut/Daten.zip", strings.Repeat("x", 120) + ".zip"} {
		name := suggestedSourceName(file)
		if !sourceNamePattern.MatchString(name) || len(name) > 80 {
			t.Errorf("%q → %q, which the stage refuses with a 422", file, name)
		}
	}
	if got := suggestedSourceName("pipedrive-export.zip"); got != "pipedrive-export" {
		t.Errorf("got %q, want the stem", got)
	}
}
