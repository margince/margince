// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extregistrytest empties the extension registries around a test.
// They are process-wide, so a test that registers a unit's declarations leaves
// them for the next test in its package unless every registry is reset.
package extregistrytest

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
)

// Reset empties the record kind, link type and signal type registries now and
// again when t ends. Company types belong to the composition, not the process.
func Reset(t testing.TB) {
	t.Helper()
	resetAll()
	t.Cleanup(resetAll)
}

func resetAll() {
	recordkinds.Reset()
	signaltypes.Reset()
}
