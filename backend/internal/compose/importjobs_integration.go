// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// ImportJobWorkers are the import kinds' workers as River runs them, each
// under the timeout its declaration gives.
type ImportJobWorkers struct {
	Commit river.Worker[ImportCommitArgs]
	Undo   river.Worker[ImportUndoArgs]
}

// ImportJobWorkersForTest builds the import workers through addImportJobs, the
// registration the worker role makes. It lives behind the integration tag, so
// no shipped binary links it.
func ImportJobWorkersForTest(pool *pgxpool.Pool, cfg JobRunnerConfig) (ImportJobWorkers, error) {
	reg := newJobRegistry()
	addImportJobs(reg, pool, cfg, slog.New(slog.DiscardHandler))
	var workers ImportJobWorkers
	if err := governedInto(reg, &workers.Commit); err != nil {
		return ImportJobWorkers{}, err
	}
	if err := governedInto(reg, &workers.Undo); err != nil {
		return ImportJobWorkers{}, err
	}
	return workers, nil
}

// WithImportJobWorkersForTest fills into with the import workers over the
// object store and upload cap of the server it composes. Passed last, it reads
// what every earlier option set.
func WithImportJobWorkersForTest(into *ImportJobWorkers, failed *error) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		*into, *failed = ImportJobWorkersForTest(pool, JobRunnerConfig{
			Blobstore: s.blobs, ImportUploadCap: s.uploadLimit,
		})
	}
}

// governedInto wraps the worker reg holds for T as addGovernedWorker does.
func governedInto[T river.JobArgs](reg *jobRegistry, into *river.Worker[T]) error {
	var zero T
	work, ok := reg.wired[zero.Kind()].worker.(jobs.WorkOnly[T])
	if !ok {
		return fmt.Errorf("compose: no %s worker is registered", zero.Kind())
	}
	spec, declared := jobs.SpecFor(zero.Kind())
	if !declared {
		return fmt.Errorf("compose: api/jobs.yaml declares no %s", zero.Kind())
	}
	*into = jobs.Govern[T](work, spec)
	return nil
}
