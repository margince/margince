// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import "time"

// Holds names the corrections an Undo still holds back. After an Undo,
// Margince does not make that correction on the deal again. The hold lifts
// when an activity, an offer or a stage move arrives after the Undo.
func Holds(lastUndone map[string]time.Time, newestEvidenceAt time.Time) map[string]bool {
	out := make(map[string]bool, len(lastUndone))
	for correction, undoneAt := range lastUndone {
		out[correction] = !newestEvidenceAt.After(undoneAt)
	}
	return out
}
