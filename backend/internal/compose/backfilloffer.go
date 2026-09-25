// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which import windows THIS installation offers, on the wire.
//
// Its own file because it answers a different question from the rest of the
// backfill transport: that maps a RUN onto the wire, and this maps the
// installation's own posture — which is why state "none" carries it too. A
// picker asks before any run exists, and that is the case the cap is for.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// offeredWindowsPayload renders the admitted set in reach order, through the
// same month→name mapping every other window on this wire uses.
//
// A month with no name is skipped rather than guessed at: the names come from
// capture's own set, so a gap means the two have drifted, and inventing "42m"
// would offer a picker a value the validator refuses.
func offeredWindowsPayload(months []int) *[]crmcontracts.BackfillStatusOfferedWindows {
	out := make([]crmcontracts.BackfillStatusOfferedWindows, 0, len(months))
	for _, m := range months {
		name, ok := windowNames[m]
		if !ok {
			continue
		}
		out = append(out, crmcontracts.BackfillStatusOfferedWindows(name))
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}
