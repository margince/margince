// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// requestOwesContext says production prepends company context to req. The
// contract says which scopes a task may be sent. A conditional policy is sent
// only to a call site that asks. A request the provider already prepared says
// it asked through the scopes it was given.
func requestOwesContext(task ai.Task, req model.Request) bool {
	policy, declared := ai.CompanyContextFor(task)
	if !declared || len(policy.Scopes) == 0 {
		return false
	}
	return !policy.Conditional || req.IncludeCompanyContext || len(req.ContextScopes) > 0
}

// ContextOwedTasks names the tasks with a site whose request production
// prepends company context to. It reads the first request each scenario's case
// builds. A task with a conditional policy no site asks for is owed nothing.
func ContextOwedTasks(ctx context.Context, corpus []Scenario, census *aitasks.Registry) (map[string]bool, error) {
	owed := map[string]bool{}
	for _, sc := range corpus {
		if owed[sc.Task] {
			continue
		}
		req, err := firstBuiltRequest(ctx, sc, census)
		if err != nil {
			return nil, err
		}
		owed[sc.Task] = requestOwesContext(ai.Task(sc.Task), req)
	}
	return owed, nil
}
