// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// insertedReadFlag emits one deal.stage_changed event and returns the
// assurance_inputs_read the outbox insert carried.
func insertedReadFlag(t *testing.T) bool {
	t.Helper()
	tx := &fakeTx{}
	if err := EmitEvent(emitTestContext(), tx, ids.NewV7(), ids.NewV7(), crmcontracts.PublicEventDealStageChanged{}); err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	decodedOutboxRow(t, tx)
	read, ok := tx.execArgs[2].(bool)
	if !ok {
		t.Fatalf("third Exec arg = %T, want bool", tx.execArgs[2])
	}
	return read
}

// Unbound, every event starts unread; bound, only a type outside the list
// starts as read.
func TestANewEventStartsUnreadUnlessTheCheckNeverReadsItsType(t *testing.T) {
	t.Cleanup(func() { UseAssuranceInputTypes(nil) })
	UseAssuranceInputTypes(nil)
	if insertedReadFlag(t) {
		t.Error("with no type list bound, a new event starts as read")
	}
	UseAssuranceInputTypes([]string{"deal.stage_changed"})
	if insertedReadFlag(t) {
		t.Error("an event of a type the check reads starts as read")
	}
	UseAssuranceInputTypes([]string{"deal.created"})
	if !insertedReadFlag(t) {
		t.Error("an event of a type the check never reads starts unread")
	}
}
