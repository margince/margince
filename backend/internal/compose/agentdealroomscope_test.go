// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealRoomWritesABuyerDoesNotRead are the agent's room writes that spend write. Each
// cuts a buyer's access or stays on the seller's side, and none changes what a buyer
// with access reads. A write missing here spends send.
var dealRoomWritesABuyerDoesNotRead = gatekit.Waive(map[string]string{
	"createDealRoom":            "nobody outside reads a new room until someone is invited, and inviting spends send",
	"archiveDealRoom":           "it ends buyer access and delivers nothing new, so a write passport may cut a buyer off",
	"pauseDealRoom":             "it suspends buyer access and delivers nothing new, so a write passport may cut a buyer off",
	"closeDealRoom":             "it freezes the content and delivers nothing new, so a write passport may stop further edits",
	"revokeDealRoomParticipant": "it takes a contact's access away and delivers nothing new, so a write passport may cut them off",
	"updateDealRoomParticipant": "it corrects the seller's record of a participant, and a buyer reads no new words from it",
	"resolveDealRoomThread":     "it settles a point and adds no words, so a buyer sees a state change and nothing written",
})

// A buyer reads a deal room live. So a write that changes what they read, or how long
// they may read it, is delivery to a counterparty. It spends send and asks the user
// first. The census is every agent write under the room's path, so a new one is held
// by being declared.
func TestADealRoomWriteABuyerReadsAtOnceSpendsSend(t *testing.T) {
	defer dealRoomWritesABuyerDoesNotRead.AssertAllMatched(t)
	sends := map[string]bool{}
	for route, pol := range agentPolicies {
		method, pattern, _ := strings.Cut(route, " ")
		if method == http.MethodGet || pol.Access != accessTool || !strings.HasPrefix(pattern, "/v1/deal-rooms") {
			continue
		}
		keepsWrite := dealRoomWritesABuyerDoesNotRead.Waived(t, pol.Op)
		switch {
		case principal.Scope(pol.Scope) == principal.ScopeSend && !keepsWrite:
			sends[pol.Op] = true
			if !pol.AskUserFirst {
				t.Errorf("%s (%s) reaches a buyer at once and does not carry ask_user_first", route, pol.Op)
			}
		case principal.Scope(pol.Scope) != principal.ScopeWrite || !keepsWrite:
			t.Errorf("%s (%s) spends %s: a room write a buyer reads spends send, and one they do not is named in this test with why", route, pol.Op, pol.Scope)
		}
	}
	for _, op := range []string{
		"updateDealRoom", "setDealRoomExpiry", "resumeDealRoom", "addDealRoomDocument", "updateDealRoomDocument", "removeDealRoomDocument",
		"openDealRoomThread", "replyDealRoomThread", "inviteDealRoomParticipant", "resendDealRoomInvitation",
	} {
		if !sends[op] {
			t.Errorf("%s does not spend send", op)
		}
	}
}
