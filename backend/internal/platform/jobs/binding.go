// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// BindWorkspace binds the installation's workspace onto the context one job
// runs under. It receives the job row so a kind that owes work to workspaces
// no company is live in (the cleanups that reach archived data) can opt out.
type BindWorkspace func(ctx context.Context, job *rivertype.JobRow) (context.Context, error)

// workerBinding is the River worker middleware that runs the binding.
type workerBinding struct {
	river.MiddlewareDefaults
	bind BindWorkspace
}

// Work binds the job's context, or fails the job without running it.
func (w *workerBinding) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	bound, err := w.bind(ctx, job)
	if err != nil {
		return FaultContext(ctx, err)
	}
	return doInner(bound)
}

// WorkerBinding is the River plugin that runs the binding. A test that works a
// job through River's test worker installs it the same way.
func WorkerBinding(bind BindWorkspace) rivertype.Plugin {
	return &workerBinding{bind: bind}
}
