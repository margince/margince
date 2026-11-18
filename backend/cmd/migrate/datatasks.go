// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
)

func runDataTasks(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return dataTaskCommand{
		operation: compose.DataTaskOperation, executor: compose.DataTaskExecute, useConfig: compose.UseConfig,
	}.run(ctx, args, stdout, stderr)
}

// dataTaskCommand holds the two ways in to the corrections, and how the command
// binds its configuration to the process. A test replaces them, to prove
// execute never reaches the operation that queues work for the worker.
type dataTaskCommand struct {
	operation func(context.Context, *pgxpool.Pool, string) ([]datatask.Progress, error)
	executor  func(context.Context, *pgxpool.Pool, int) (compose.DataTaskRun, error)
	useConfig func(*config.Config)
}

// dataTaskArgs is one data-tasks command line, checked.
type dataTaskArgs struct {
	action     string
	dsn        *dsnSettings
	wait       bool
	maxBatches int
}

func parseDataTaskArgs(args []string, stderr io.Writer) (dataTaskArgs, error) {
	if len(args) == 0 {
		return dataTaskArgs{}, errors.New("usage: migrate data-tasks <status|estimate|run|execute> [--wait] [--max-batches n] --dsn <dsn>")
	}
	action := args[0]
	if action != "status" && action != "estimate" && action != "run" && action != dataTasksExecute {
		return dataTaskArgs{}, fmt.Errorf("unknown data-tasks operation %q", action)
	}
	fs := flag.NewFlagSet("data-tasks", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dsn, err := addDSNFlags(fs)
	if err != nil {
		return dataTaskArgs{}, err
	}
	wait := fs.Bool("wait", false, "wait for background corrections to finish (run only)")
	maxBatches := fs.Int("max-batches", 100000, "the most batches execute commits before it reports (execute only)")
	if err := fs.Parse(args[1:]); err != nil {
		return dataTaskArgs{}, config.FlagFault(fs, err)
	}
	if *maxBatches < 1 {
		return dataTaskArgs{}, errors.New("--max-batches must be at least 1")
	}
	if *wait && action != "run" {
		return dataTaskArgs{}, errors.New("--wait requires data-tasks run")
	}
	return dataTaskArgs{action: action, dsn: dsn, wait: *wait, maxBatches: *maxBatches}, nil
}

func (c dataTaskCommand) run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	parsed, err := parseDataTaskArgs(args, stderr)
	if err != nil {
		return err
	}
	action, dsn := parsed.action, parsed.dsn
	resolved, err := dsn.resolve(config.Environ())
	if err != nil {
		return err
	}
	pool, err := database.NewPool(ctx, resolved)
	if err != nil {
		return err
	}
	defer pool.Close()
	// The corrections hash under the keys the root key derives, as the worker's do.
	c.useConfig(dsn.loaded)
	ctx = compose.ConfigContext(ctx, dsn.loaded)
	if err := storekit.RefuseForeignSuppressionKey(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if action == dataTasksExecute {
		blob, _, err := blobstore.FromConfig(ctx, dsn.loaded)
		if err != nil {
			return fmt.Errorf("migrate: open original storage: %w", err)
		}
		ctx = originalstore.Bind(ctx, compose.InstallationDB(pool), blob)
		return c.execute(ctx, pool, parsed.maxBatches, stdout)
	}
	progress, err := c.operation(ctx, pool, action)
	if err != nil {
		return err
	}
	if parsed.wait {
		progress, err = c.wait(ctx, pool, progress)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(stdout).Encode(compose.DataTaskProgressReport(progress))
}

func dataTasksFinished(progress []datatask.Progress) (bool, error) {
	done := true
	for _, p := range progress {
		if p.AwaitingReview() {
			continue
		}
		if p.BlockedReason == "missing_prerequisite" || p.BlockedReason == "newer_release_required" {
			return false, fmt.Errorf("correction %s step %s is blocked by %s; review System health", p.TaskID, p.Step, p.BlockedReason)
		}
		if p.Status == datatask.Failed || p.Status == datatask.Paused {
			return false, fmt.Errorf("correction %s step %s is %s; review System health", p.TaskID, p.Step, p.Status)
		}
		if p.Status != datatask.Done {
			done = false
		}
	}
	return done, nil
}

func (c dataTaskCommand) wait(ctx context.Context, pool *pgxpool.Pool, progress []datatask.Progress) ([]datatask.Progress, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		done, err := dataTasksFinished(progress)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		progress, err = c.operation(ctx, pool, "status")
		if err != nil {
			return nil, err
		}
	}
	return progress, nil
}

// dataTasksExecute runs the corrections in this process instead of queueing
// them for the worker, for a copy whose worker must not start.
const dataTasksExecute = "execute"

// dataTasksReport is what execute prints: the verdict, then every step.
type dataTasksReport struct {
	Verdict compose.DataTaskVerdict         `json:"verdict"`
	Steps   []crmcontracts.DataTaskProgress `json:"steps"`
}

// execute prints the report. It fails unless every step finished and none left
// rows in its scope, so a script cannot read either as done.
func (c dataTaskCommand) execute(ctx context.Context, pool *pgxpool.Pool, maxBatches int, stdout io.Writer) error {
	run, err := c.executor(ctx, pool, maxBatches)
	if err != nil {
		return err
	}
	verdict := compose.DataTaskVerdictOf(run)
	if err := json.NewEncoder(stdout).Encode(dataTasksReport{Verdict: verdict, Steps: compose.DataTaskProgressReport(run.Progress)}); err != nil {
		return err
	}
	switch {
	case !verdict.Finished:
		return errors.New("data-tasks execute: not every correction finished; read the verdict")
	case len(verdict.LeftInScope) > 0:
		return errors.New("data-tasks execute: corrections left rows in their scope; read left_in_scope")
	}
	return nil
}
