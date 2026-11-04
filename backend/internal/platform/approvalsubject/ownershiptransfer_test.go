// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARequesterIsOnlyReadFromABodyThatIsATransfer(t *testing.T) {
	giver, receiver, stranger := ids.NewV7(), ids.NewV7(), ids.NewV7()
	body := func(transfer OwnershipTransfer) json.RawMessage {
		raw, err := json.Marshal(transfer)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	whole := OwnershipTransfer{FromID: giver, ToID: receiver, RequestedBy: giver, DeciderID: receiver}
	if got := OwnershipTransferRequester(body(whole)); got != giver {
		t.Errorf("requester of a whole transfer = %s, want %s", got, giver)
	}
	for name, broken := range map[string]OwnershipTransfer{
		"only a requester":            {RequestedBy: giver},
		"a requester who is no party": {FromID: giver, ToID: receiver, RequestedBy: stranger, DeciderID: receiver},
		"one party on both sides":     {FromID: giver, ToID: giver, RequestedBy: giver, DeciderID: giver},
	} {
		if got := OwnershipTransferRequester(body(broken)); got != ids.Nil {
			t.Errorf("%s: requester = %s, want zero", name, got)
		}
	}
}
