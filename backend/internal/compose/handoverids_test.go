// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// An omitted to_user_id is refused by name, not read as a colleague who may
// not own records.
func TestEveryRequiredHandoverBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	_, err := handoverReceiver(crmcontracts.HandOverUserWorkRequest{})
	if err == nil || !strings.Contains(err.Error(), "to_user_id") {
		t.Fatalf("an omitted to_user_id → %v, want a refusal naming it", err)
	}
}
