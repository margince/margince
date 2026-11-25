// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func dataTaskMetricsSection(pool *pgxpool.Pool) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) error {
		ctx, engine, err := dataTaskJobEngine(ctx, pool)
		if err != nil {
			return failedDataTaskRead(ctx, w, err)
		}
		progress, err := engine.List(ctx)
		if err != nil {
			return failedDataTaskRead(ctx, w, err)
		}
		if err := writeDataTaskRead(w, true); err != nil {
			return err
		}
		return writeDataTaskMetrics(w, progress, time.Now())
	}
}

func failedDataTaskRead(ctx context.Context, w io.Writer, cause error) error {
	slog.ErrorContext(ctx, "metrics: background correction progress read failed", "err", cause)
	return writeDataTaskRead(w, false)
}

func writeDataTaskRead(w io.Writer, ok bool) error {
	if err := writeFamilyHeader(w, "margince_data_task_read_success", "Whether correction progress could be read."); err != nil {
		return err
	}
	value := 0
	if ok {
		value = 1
	}
	_, err := fmt.Fprintf(w, "margince_data_task_read_success %d\n", value)
	return err
}

func writeDataTaskMetrics(w io.Writer, progress []datatask.Progress, now time.Time) error {
	counts := map[string]int{}
	for _, p := range progress {
		counts[p.Status]++
	}
	if err := writeFamilyHeader(w, "margince_data_task_steps", "Registered correction steps by status."); err != nil {
		return err
	}
	for _, status := range []string{datatask.Pending, datatask.Running, datatask.Paused, datatask.Done, datatask.Failed} {
		if _, err := fmt.Fprintf(w, "margince_data_task_steps{status=%s} %d\n", label(status), counts[status]); err != nil {
			return err
		}
	}
	if err := writeFamilyHeader(w, "margince_data_task_seconds_since_progress", "Seconds since the last committed correction batch; absent before any batch."); err != nil {
		return err
	}
	for _, p := range progress {
		if p.LastProgressAt == nil {
			continue
		}
		if _, err := fmt.Fprintf(w, "margince_data_task_seconds_since_progress{task=%s,step=%s} %g\n", label(p.TaskID), label(p.Step), max(0, now.Sub(*p.LastProgressAt).Seconds())); err != nil {
			return err
		}
	}
	return nil
}
