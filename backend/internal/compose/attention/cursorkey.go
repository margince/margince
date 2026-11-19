// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"crypto/rand"
	"sync"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// cursorKeyLabel derives the key worklist cursors are signed with.
const cursorKeyLabel = "margince worklist cursor v1"

// processCursorKey stands in where no root key is configured. A cursor then
// resumes only on the process that minted it, and any other process refuses it
// as the caller's mistake instead of trusting an unsigned one.
var processCursorKey = sync.OnceValue(func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("attention: reading random bytes for the cursor key: " + err.Error())
	}
	return key
})

func cursorKey() []byte {
	if key := keyvault.DerivedKey(config.Bound(), cursorKeyLabel); key != nil {
		return key
	}
	return processCursorKey()
}
