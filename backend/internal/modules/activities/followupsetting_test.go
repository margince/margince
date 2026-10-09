// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"testing"
)

func TestTheFollowUpWindowIsOneToThirtyDays(t *testing.T) {
	t.Parallel()
	for days, admit := range map[int]bool{0: false, 1: true, 2: true, 30: true, 31: false} {
		raw, err := json.Marshal(days)
		if err != nil {
			t.Fatal(err)
		}
		err = FollowUpAfterDays.ValidateJSON(raw)
		if admit && err != nil {
			t.Errorf("%d days refused: %v", days, err)
		}
		if !admit && err == nil {
			t.Errorf("%d days admitted, want refused", days)
		}
	}
}
