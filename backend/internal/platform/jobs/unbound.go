// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"strings"

	"github.com/riverqueue/river"
)

// dependsOnModel says whether a registration gate names an AI lane. The
// declaration already states this: a kind gated on a Brain, the Embedder or
// the agent scheduler does model work.
func dependsOnModel(when []string) bool {
	for _, field := range when {
		if strings.HasSuffix(field, "Brain") || field == "Embedder" || field == "AgentScheduler.Service" {
			return true
		}
	}
	return false
}

type quietWorker[T river.JobArgs] struct {
	work  WorkOnly[T]
	bound func() bool
}

func (w quietWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	if !w.bound() {
		return nil
	}
	return w.work.Work(ctx, job)
}

// QuietWhileUnbound completes a model job as a no-op while no model is bound.
// It applies to a kind that registers nothing without its lane: a role booted
// with no model never ran it, and a bound-later role must not start failing it
// first. A kind that registers anyway is a request somebody queued and still
// runs, so it fails with its reason. A nil bound leaves the worker alone.
func QuietWhileUnbound[T river.JobArgs](w WorkOnly[T], s Spec, bound func() bool) WorkOnly[T] {
	if bound == nil || s.Registration.AbsentRegistersAnyway || !dependsOnModel(s.Registration.When) {
		return w
	}
	return quietWorker[T]{work: w, bound: bound}
}
