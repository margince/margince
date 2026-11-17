// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/draftreason"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// draftedReason is what a drafting instruction says the message is. An ask to
// move an appointment is a follow-up on it. A meeting anchor is its evidence. Nothing otherwise, because silence
// leaves the reason to the thread or to the sender.
func draftedReason(intent string, anchor ids.UUID, kind crmcontracts.ActivityKind) (commsauthz.Category, commsauthz.Evidence) {
	if draftreason.ForInstruction(intent) == "" {
		return "", commsauthz.Evidence{}
	}
	if kind == crmcontracts.ActivityKindMeeting {
		return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{ActivityID: anchor}
	}
	return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{}
}
