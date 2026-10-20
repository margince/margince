// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type selection struct {
	task     Task
	step     Step
	progress Progress
}

// An earlier paused or failed correction blocks later automatic release work.
func nextStep(tasks []Task, progress map[string]Progress, now time.Time) (*selection, string) {
	for _, task := range tasks {
		for _, step := range task.Steps {
			p, exists := progress[progressKey(task.ID, step.Name)]
			if task.Manual && p.Activation == nil {
				continue
			}
			if complete(task, p) {
				continue
			}
			candidate := &selection{task: task, step: step, progress: p}
			if !exists {
				return candidate, "waiting_for_worker"
			}
			if task.Manual && (p.TaskVersion != task.Version || p.Activation.RuleVersion != task.ReviewVersion) {
				return candidate, ReviewRequired
			}
			if p.TaskVersion > task.Version {
				return candidate, "newer_release_required"
			}
			if p.Status == Paused || p.Status == Failed {
				return candidate, p.Status
			}
			if p.NextAttemptAt != nil && now.Before(*p.NextAttemptAt) {
				return candidate, "retry_wait"
			}

			return &selection{task: task, step: step, progress: p}, ""
		}
	}
	return nil, ""
}

func missingRequirements(ctx context.Context, tx pgx.Tx, task Task) ([]string, error) {
	if len(task.Requires) == 0 {
		return nil, nil
	}
	versions := make([]string, len(task.Requires))
	for i, requirement := range task.Requires {
		versions[i] = strings.TrimPrefix(requirement, "core/")
	}
	rows, err := tx.Query(ctx, `SELECT version FROM schema_migrations_core WHERE version = ANY(@versions)`, pgx.NamedArgs{"versions": versions})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[string]bool, len(versions))
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		found[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, version := range versions {
		if !found[version] {
			missing = append(missing, "core/"+version)
		}
	}
	return missing, nil
}

func complete(task Task, p Progress) bool { return p.TaskVersion == task.Version && p.Status == Done }
