// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RetentionPassCtx is the scope the retention workspace worker binds before it
// calls the engine: the tenant, the system actor, and a fresh correlation id.
// The engine writes an audit row and an outbox event per record it retires, so a
// suite that bound only the workspace would be exercising a pass whose provenance
// production never has. It is the counterpart to harness.go's As, which builds the
// scope a HUMAN request arrives with.
//
// The actor is a SYSTEM principal, which no row-scope clause narrows, so this
// scope cannot be denied — which makes it the wrong instrument for any assertion
// about who may SEE a row. Such a test would pass whatever the row scope was. That
// is not left to this comment: backend/gates/retentionscope_test.go holds the fixture to
// the retention engine as its only sink, and fails a use that is anything else.
// It calls principal.SystemActing, which is what the worker calls, so these
// suites now hold the pass's own binding and not only the engine beneath it:
// delete the actor from that helper and they go red. They could not call the
// worker's own spelling while there was one — internal/compose has in-package
// integration tests that import this package, so importing compose back from a
// non-test file here is an import cycle — which is why the shape moved down to
// the kernel rather than the fixture moving up.
func RetentionPassCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	return principal.SystemActing(ctx, "system")
}
