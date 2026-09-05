// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// ImportRowHolds answers whether ONE importing seat's row, on its own, still
// asks for the message to be held.
//
// It is contributionOf restated as a boolean, and exists so that the seam
// reporting "how many colleagues still hold this" counts by the same rule this
// module derives the audience from. The rule's non-obvious half is that an
// explicit verdict outranks the posture in BOTH directions: a mailbox that was
// `classified` at import holds only until its seat decides, so a seat whose
// verdict is `shared_by_owner` or `cleared` is not holding, whatever posture
// the message arrived under. A count that read the posture as a standing hold
// told the last owner to share a two-mailbox message that it was still private
// at the moment the recompute opened it to the workspace.
func ImportRowHolds(posture, status *string) bool {
	audience, _ := contributionOf(posture, status, nil)
	return audience != audienceWorkspace
}
