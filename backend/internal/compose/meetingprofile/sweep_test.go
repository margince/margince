// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingprofile

import "testing"

// The worker builds Sweep with no Batch, so it takes 200 rows a transaction.
func TestTheSweepTakesTwoHundredRowsATransactionByDefault(t *testing.T) {
	if got := (Sweep{}).batch(); got != 200 {
		t.Fatalf("the default sweep batch is %d, want 200", got)
	}
}
