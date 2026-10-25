// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// foldLastContact keeps the activity the tile names, with its subject when
// the page carries the row.
func foldLastContact(view crmcontracts.Company360, in *Input) {
	last := view.LastContact
	if last == nil {
		return
	}
	act := ActIn{
		ID:   last.ActivityId.String(),
		Kind: string(last.Kind),
		At:   occurredday.When(occurredday.Moment{At: last.At, DateOnly: occurredday.IsDay(last.OccurredPrecision)}),
	}
	if view.Activities != nil {
		for _, activity := range view.Activities.Data {
			if activity.Id == last.ActivityId && activity.Subject != nil {
				act.Subject = *activity.Subject
			}
		}
	}
	in.LastContact = &act
}

// lastContact is the activity the page names as last contact. A note, a task
// or a meeting called off sits on the timeline too, and is not contact.
func lastContact(in Input) (ActIn, bool) {
	if in.LastContact == nil {
		return ActIn{}, false
	}
	return *in.LastContact, true
}
