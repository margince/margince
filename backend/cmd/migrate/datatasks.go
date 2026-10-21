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
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func runDataTasks(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: migrate data-tasks <status|estimate|run> [--wait] --dsn <dsn>")
	}
	action := args[0]
	if action != "status" && action != "estimate" && action != "run" {
		return fmt.Errorf("unknown data-tasks operation %q", action)
	}
	fs := flag.NewFlagSet("data-tasks", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dsn, err := addDSNFlags(fs)
	if err != nil {
		return err
	}
	wait := fs.Bool("wait", false, "wait for background corrections to finish (run only)")
	if err := fs.Parse(args[1:]); err != nil {
		return config.FlagFault(fs, err)
	}
	if *wait && action != "run" {
		return errors.New("--wait requires data-tasks run")
	}
	resolved, err := dsn.resolve(config.Environ())
	if err != nil {
		return err
	}
	pool, err := database.NewPool(ctx, resolved)
	if err != nil {
		return err
	}
	defer pool.Close()
	progress, err := compose.DataTaskOperation(ctx, pool, action)
	if err != nil {
		return err
	}
	if *wait {
		progress, err = waitForDataTasks(ctx, pool, progress)
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

func waitForDataTasks(ctx context.Context, pool *pgxpool.Pool, progress []datatask.Progress) ([]datatask.Progress, error) {
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
		progress, err = compose.DataTaskOperation(ctx, pool, "status")
		if err != nil {
			return nil, err
		}
	}
	return progress, nil
}
