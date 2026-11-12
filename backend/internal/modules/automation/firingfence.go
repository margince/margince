// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// FiringFence keeps legacy effects outside an intake or policy cutover.
type FiringFence interface {
	Within(context.Context, workflow.Spec, workflow.Event, func(context.Context) error) error
}

func (e *WorkflowEngine) WithFiringFence(fence FiringFence) *WorkflowEngine {
	e.fence = fence
	return e
}
