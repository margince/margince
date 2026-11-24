// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivitySourceAudienceArm is ActivityAudienceArm for a row that is no task
// standing for a source message, such as the message a reply channel reads. It
// refuses a task row and drops the task hops, which would plan a second full
// message gate into every statement that carries it.
func ActivitySourceAudienceArm(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if p.Type == principal.PrincipalSystem {
		return ActivityAudienceArm(ctx, alias, arg)
	}
	return taskFreeSQL(alias) + " AND " + activityAudienceArm(p, alias, arg) + agentRestrictionArm(p, alias), nil
}

// ActivityReadSourceAudienceArm is ActivitySourceAudienceArm for the source of
// a task whose content the caller may read. It renders nothing for a human or
// an agent. Their content gate's evidence test already ran the message content
// gate on that source, and that gate holds every term of the arm. The system
// principal's gate tests no evidence, so it keeps the arm.
func ActivityReadSourceAudienceArm(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if p.Type == principal.PrincipalSystem {
		return ActivitySourceAudienceArm(ctx, alias, arg)
	}
	return "", nil
}
