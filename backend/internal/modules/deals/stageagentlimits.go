// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refuseAgentSemanticChange keeps which stages close a deal with a user.
// Automatic work needs approval to move a deal into a won or lost stage. An agent
// that could relabel a stage, or add a closing one, would lift that for every agent
// working unattended.
func refuseAgentSemanticChange(ctx context.Context, current string, wanted *string) error {
	if wanted == nil || *wanted == current {
		return nil
	}
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	return &httperr.DetailedError{
		Status: http.StatusForbidden, Code: "agent_sets_no_stage_semantic",
		Detail: "an agent does not decide whether a stage counts as won or lost; ask the user to do it in the app",
	}
}

// stagePositionTaken says which position is occupied and what frees it.
func stagePositionTaken(position int) error {
	return &httperr.DetailedError{
		Status: http.StatusConflict, Code: "stage_position_taken",
		Detail: fmt.Sprintf("position %d is already taken in this pipeline; choose a free position, or reorder the stages to put this one there", position),
	}
}
