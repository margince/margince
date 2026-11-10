// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// WithholdContact adds the contact's attach rights to the shared action gate.
func WithholdContact(ctx context.Context, language string, moment *crmcontracts.ContactMoment, attachable bool) {
	Withhold(ctx, language, moment)
	if attachable {
		return
	}
	reason := langcopy.For(language).Say(blockedWords.ReadOnlyContact)
	block := func(action *crmcontracts.ContactMomentAction) {
		if action.Destination == nil {
			return
		}
		switch action.Destination.Surface {
		case crmcontracts.ContactMomentDestinationSurfaceActivityLog, crmcontracts.ContactMomentDestinationSurfaceBooking,
			crmcontracts.ContactMomentDestinationSurfaceComposer:
			blockAction(action, reason)
		}
	}
	block(&moment.RecommendedAction)
	if moment.SecondaryActions != nil {
		for i := range *moment.SecondaryActions {
			block(&(*moment.SecondaryActions)[i])
		}
	}
}
