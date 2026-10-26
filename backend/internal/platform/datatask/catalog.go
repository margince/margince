// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Querier keeps estimates inside the executor's bounded transaction.
type Querier = dbmigrate.Querier

// AuditPolicy allows an unaudited derived column only with its runtime precedent.
type AuditPolicy struct {
	PerRecord bool
	Reason    string
}

// Task is release code whose durable ID survives skipped upgrades.
type Task struct {
	ID            string
	Version       int
	Title         string
	Requires      []string
	Steps         []Step
	Manual        bool
	ReviewVersion string
	Review        func(context.Context, pgx.Tx, ReviewRequest) (string, error)
}

// Step keeps the domain writer and its progress in one transaction.
type Step struct {
	Name     string
	Limit    int
	MinLimit int
	Audit    AuditPolicy
	Estimate func(context.Context, Querier, int64) (int64, bool, error)
	Batch    func(context.Context, pgx.Tx, BatchIn) (BatchOut, error)
	// EmptyWhenDone marks an estimate that counts only rows still to correct,
	// so a finished step that still estimates rows left some uncorrected.
	EmptyWhenDone bool
}

// BatchIn retains the run attribution across retries and restarts.
type BatchIn struct {
	Cursor     json.RawMessage
	Limit      int
	RunID      ids.UUID
	Activation *Activation
}

// BatchOut advances over scanned rows even when no row needed a correction.
type BatchOut struct {
	Cursor  json.RawMessage
	Scanned int
	Changed int
	Done    bool
}

var (
	taskIDPattern      = regexp.MustCompile(`^core/[0-9]+_[a-z][a-z0-9_]*$`)
	requirementPattern = regexp.MustCompile(`^core/[0-9]+$`)
	stepNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Catalog validates registration before a worker can seed durable progress.
func Catalog(tasks []Task) ([]Task, error) {
	out := slices.Clone(tasks)
	slices.SortFunc(out, func(a, b Task) int { return cmp.Compare(a.ID, b.ID) })
	for i, task := range out {
		if !ValidTaskID(task.ID) || task.Version < 1 || task.Title == "" || len(task.Steps) == 0 {
			return nil, fmt.Errorf("data task registration is incomplete: %q", task.ID)
		}
		if i > 0 && out[i-1].ID == task.ID {
			return nil, fmt.Errorf("data task is registered twice: %s", task.ID)
		}
		if task.Manual && (task.ReviewVersion == "" || task.Review == nil) {
			return nil, fmt.Errorf("manual data task %s requires a versioned review", task.ID)
		}
		for _, requirement := range task.Requires {
			if !requirementPattern.MatchString(requirement) {
				return nil, fmt.Errorf("data task %s has an invalid prerequisite", task.ID)
			}
		}
		if err := validateSteps(task); err != nil {
			return nil, err
		}
		out[i].Requires = slices.Clone(task.Requires)
		out[i].Steps = slices.Clone(task.Steps)
	}
	return out, nil
}

func validateSteps(task Task) error {
	names := make(map[string]bool, len(task.Steps))
	for _, step := range task.Steps {
		if !stepNamePattern.MatchString(step.Name) || names[step.Name] ||
			step.MinLimit < 1 || step.Limit < step.MinLimit || step.Batch == nil || step.Estimate == nil {
			return fmt.Errorf("data task %s has an invalid bounded step: %s", task.ID, step.Name)
		}
		if step.Audit.PerRecord == (step.Audit.Reason != "") {
			return fmt.Errorf("data task %s must declare one audit policy for %s", task.ID, step.Name)
		}
		names[step.Name] = true
	}
	return nil
}

// ValidTaskID shares the registration grammar with release-history checks.
func ValidTaskID(id string) bool { return taskIDPattern.MatchString(id) }
