// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "github.com/margince/margince/backend/internal/shared/kernel/threadverdict"

// DealEvidenceSQL admits an activity that may speak for a deal being alive. It
// refuses only what the product already decided is personal: a thread a seat
// holds as personal, or a row whose audience reason says so. A calendar
// privacy mark decides nothing, since business meetings carry one too. The
// deal clock's migration carries this text.
func DealEvidenceSQL(alias string) string {
	return "(coalesce(" + alias + ".audience_reason, '') <> '" +
		threadverdict.ReasonForKind(threadverdict.KindPersonal) + "' AND " + threadverdict.NoPersonalHoldSQL(alias) + ")"
}
