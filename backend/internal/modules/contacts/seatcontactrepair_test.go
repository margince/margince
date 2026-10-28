// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contact step archives on the system's authority alone, and its count
// needs the contact read grant. Both refuse before they read anything.
func TestTheSeatContactRepairRefusesAnyoneButTheSystem(t *testing.T) {
	if _, err := ArchiveSeatContactsTx(context.Background(), nil, ids.Nil, 10); err == nil {
		t.Error("a caller with no principal archived seat contacts")
	}
	if _, err := CountLiveContacts(context.Background(), nil, 10); err == nil {
		t.Error("a caller with no principal counted the live contacts")
	}
}
