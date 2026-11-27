// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"crypto/sha256"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// cursorKeyLabel derives the key worklist cursors are signed with.
const cursorKeyLabel = "margince worklist cursor v1"

// unkeyedCursorKey stands in where the installation sets no root key. It is
// the same on every replica and across restarts, so a cursor resumes anywhere.
// It is public, so there a cursor only detects accidental edits. A root key
// makes it authenticate, as it does for preference links.
var unkeyedCursorKey = sha256.Sum256([]byte(cursorKeyLabel + " unkeyed"))

func cursorKey() []byte {
	return cursorKeyFrom(keyvault.DerivedKey(config.ProcessBound(), cursorKeyLabel))
}

// cursorKeyFrom is the derived key, or the shared fallback when none is set.
func cursorKeyFrom(derived []byte) []byte {
	if derived != nil {
		return derived
	}
	return unkeyedCursorKey[:]
}
