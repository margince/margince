// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"strings"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// requestedModelWork names the gates of kinds somebody queued on purpose. While
// no model is bound they still run and fail with their reason, because a
// request that completes without doing anything reads as work that was done.
var requestedModelWork = map[string]bool{
	"DeepReadBrain": true, "DocumentExtractBrain": true, "TranscriptProposeBrain": true,
	"AccountScanBrain": true, "VoiceBrain": true,
}

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

func isRequestedModelWork(when []string) bool {
	for _, field := range when {
		if requestedModelWork[field] {
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

// QuietWhileUnbound completes a gated model job as a no-op while no model is
// bound, with no error for River to retry. A nil bound leaves the worker alone.
// Kinds the declaration does not gate on a model lane are covered after the
// call by quietOnUnboundRefusal.
func QuietWhileUnbound[T river.JobArgs](w WorkOnly[T], s Spec, bound func() bool) WorkOnly[T] {
	if bound == nil || isRequestedModelWork(s.Registration.When) || !dependsOnModel(s.Registration.When) {
		return w
	}
	return quietWorker[T]{work: w, bound: bound}
}

// quietOnUnboundRefusal turns the refusal an unbound router gives into a clean
// completion, for a job that reached a model call. Requested model work keeps
// its failure.
func quietOnUnboundRefusal(err error, s Spec) error {
	if errors.Is(err, httperr.ErrNotConfigured) && !isRequestedModelWork(s.Registration.When) {
		return nil
	}
	return err
}
