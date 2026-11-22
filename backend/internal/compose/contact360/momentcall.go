// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// callsBack makes the call the card's lead move. It applies when the owed
// task was filed from a logged call and the contact has a number to dial.
// Writing stays as the second move.
//
// The reply channel is the kind of the task's source activity, never its
// title. It is null for a source this reader may not see. The screen and an
// assistant reading the moment over MCP therefore get one answer.
func (said momentCopy) callsBack(moment *crmcontracts.ContactMoment, task crmcontracts.Activity, contact crmcontracts.Contact) {
	if task.ReplyChannel == nil || *task.ReplyChannel != crmcontracts.ActivityReplyChannelCall ||
		contact.ArchivedAt != nil || !hasDialableNumber(contact) {
		return
	}
	write := moment.RecommendedAction
	moment.RecommendedAction = crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindCall,
		Label: said.Say(momentWords.Call),
		State: crmcontracts.ContactMomentActionStateAvailable,
	}
	moment.SecondaryActions = &[]crmcontracts.ContactMomentAction{write}
}

// hasDialableNumber mirrors the client's primaryPhone: a listed number that is
// not blank. The contact read already leaves out archived numbers.
func hasDialableNumber(contact crmcontracts.Contact) bool {
	if contact.Phones == nil {
		return false
	}
	for _, number := range *contact.Phones {
		if number.Phone != "" {
			return true
		}
	}
	return false
}
