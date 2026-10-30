// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actorPrefix starts the actor every task's audit rows name.
const actorPrefix = "system:datatask:"

// Acting keeps the release task identity in every domain audit written by its batch.
func Acting(ctx context.Context, taskID string) context.Context {
	return principal.SystemActing(ctx, actorPrefix+taskID)
}

// ActorID is the actor a task's audit rows name, so a later task can find
// what an earlier one wrote.
func ActorID(taskID string) string {
	return actorPrefix + taskID
}
