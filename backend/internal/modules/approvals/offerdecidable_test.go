// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Only the receiver an ownership offer names is spared the write check on the
// record. The giver deciding a request, another member, and any other kind
// naming the reader keep it.
func TestOnlyTheReceiverOfAnOwnershipOfferIsSparedTheTargetWriteCheck(t *testing.T) {
	giver, receiver, stranger := ids.NewV7(), ids.NewV7(), ids.NewV7()
	transfer := func(requestedBy, decider ids.UUID) json.RawMessage {
		body, err := json.Marshal(approvalsubject.OwnershipTransfer{
			RecordType: "company", RecordID: ids.NewV7(), FromID: giver, ToID: receiver,
			RequestedBy: requestedBy, DeciderID: decider,
		})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	share, err := json.Marshal(approvalsubject.ContactShare{ContactID: ids.NewV7(), OwnerID: receiver, RequesterID: giver})
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		kind   string
		body   json.RawMessage
		reader ids.UUID
		want   bool
	}{
		"the receiver of an offer":       {approvalsubject.KindOwnershipTransfer, transfer(giver, receiver), receiver, true},
		"the giver deciding a request":   {approvalsubject.KindOwnershipTransfer, transfer(receiver, giver), giver, false},
		"the receiver of a request":      {approvalsubject.KindOwnershipTransfer, transfer(receiver, giver), receiver, false},
		"another member":                 {approvalsubject.KindOwnershipTransfer, transfer(giver, receiver), stranger, false},
		"an offer naming no decider":     {approvalsubject.KindOwnershipTransfer, transfer(giver, giver), receiver, false},
		"a share request put to them":    {approvalsubject.KindContactShare, share, receiver, false},
		"an offer's payload, other kind": {"assign_owner", transfer(giver, receiver), receiver, false},
	} {
		t.Run(name, func(t *testing.T) {
			p := principal.Principal{Type: principal.PrincipalHuman, UserID: c.reader}
			if got := offeredToReader(p, row{Kind: c.kind, ProposedChange: c.body}); got != c.want {
				t.Errorf("spared = %v, want %v", got, c.want)
			}
		})
	}
}
