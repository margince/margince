// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Acting keeps the release task identity in every domain audit written by its batch.
func Acting(ctx context.Context, taskID string) context.Context {
	return principal.SystemActing(ctx, ActorID(taskID))
}

// ActorID is the principal a release task's batches bind, so a module can
// recognise what the task wrote.
func ActorID(taskID string) string {
	return "system:datatask:" + taskID
}
