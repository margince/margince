// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withAmbientIdentity stamps what a trace row is joined on from the call's
// context: the correlation id, the agent run, the subject and the extension.
// It also carries whether the site serves a floor when the call fails.
// Completion and decision attempts both read them here, so one logical call
// cannot file its rows under two correlations.
func withAmbientIdentity(ctx context.Context, trace *Call) {
	if cid, ok := principal.CorrelationID(ctx); ok {
		trace.CorrelationID = &cid
	}
	if rid, ok := principal.AgentRunID(ctx); ok {
		trace.AgentRunID = &rid
	}
	if subject, ok := SubjectOf(ctx); ok {
		trace.Subject = subject
	}
	trace.Extension = extensionOf(ctx)
	trace.FloorServed = ServesFloor(ctx)
}
