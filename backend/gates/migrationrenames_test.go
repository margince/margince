// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// It carries NO build constraint, for the reason repoRoot beside it does not:
// a helper behind `//go:build !integration` is invisible to `make lint`, which
// sets that tag, so a gate using it would compile under `go test` and fail to
// compile under the linter.

import (
	"path/filepath"
	"sync"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// withCurrentNames is gatekit.WithCurrentNames over this tree's core
// migrations, which owns the rename derivation for every census that reads
// migration text.
//
// Remembered per input: the schema gates rewrite the same migrations, and the
// rename map cannot change during a run.
func withCurrentNames(sql string) string {
	if done, ok := currentNameRewrites.Load(sql); ok {
		if rewritten, isString := done.(string); isString {
			return rewritten
		}
	}
	rewritten := gatekit.WithCurrentNames(filepath.Join(repoRoot, "backend", "migrations", "core"), sql)
	currentNameRewrites.Store(sql, rewritten)
	return rewritten
}

var currentNameRewrites sync.Map // migration SQL -> withCurrentNames of it
