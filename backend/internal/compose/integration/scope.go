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
// It is a COPY of the worker's own retentionPassProvenance rather than a call
// to it, and it cannot be one: internal/compose has in-package integration
// tests that import this package, so importing compose back from a non-test
// file here is an import cycle. What that costs is that these suites hold the
// retention ENGINE under a pass's provenance and not the pass's own binding —
// deleting that line from the worker leaves them green. The binding is held
// instead by TestEveryJobWorkerThatReachesAStoreBindsAnActor, which reads the
// worker; #4952 is the one-source-of-truth follow-up.
func RetentionPassCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
