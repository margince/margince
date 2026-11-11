// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// SignatureWindow is the run of sender lines a signature can sit in, at most
// maxLines long. It opens at the last sign-off or the first sig-dash, whichever
// comes first, and keeps the head of a longer run.
//
// The head, because a signature puts its name, title and numbers at the top and
// its legal footer at the foot. Without either boundary the window is the tail.
func SignatureWindow(lines []string, maxLines int) []string {
	start := signatureBoundary(lines)
	if start < 0 {
		return lines[max(0, len(lines)-maxLines):]
	}
	return lines[start:min(len(lines), start+maxLines)]
}

// signatureBoundary is the index the signature opens at, or -1. The last
// sign-off, not the first: a "Thanks!" above the message would open it too early.
func signatureBoundary(lines []string) int {
	signOff, dash := -1, -1
	for i, line := range lines {
		if isClosing(line) {
			signOff = i
		}
		if dash < 0 && textlang.IsSigDash(line) {
			dash = i
		}
	}
	switch {
	case signOff < 0:
		return dash
	case dash < 0:
		return signOff
	}
	return min(signOff, dash)
}
