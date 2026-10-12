// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Certifying one task on each of its candidates: the rung that answers first,
// then each fallback, a record per model, and the fallbacks the judge cannot
// grade named at both ends of the run.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// certifyAndWrite certifies one task on each of its candidates and writes a
// record per candidate as soon as it is certified, so a fallback that fails
// never costs the task the record of the rung that answers.
func certifyAndWrite(ctx context.Context, cfg RunnerConfig, task ai.Task, scenarios []Scenario, repeats int,
	trace *payloadTrace, journal *runJournal, log *slog.Logger,
) ([]Record, error) {
	cands, _, err := taskCandidates(cfg, task)
	if err != nil {
		return nil, err
	}
	var written []Record
	var errs []error
	for _, c := range cands {
		recs, err := certifyCandidate(ctx, cfg, task, scenarios, repeats, trace, journal, c, log)
		written = append(written, recs...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return written, errors.Join(errs...)
}

// certifyCandidate certifies one task on one candidate and writes its records:
// the LLM record, then — on the rung that answers, when the run binds a
// decisions lane — each decision site's beside it. It returns what it wrote even
// when a later step failed, so a decision leg that fails never costs the task
// the LLM record already on disk.
func certifyCandidate(ctx context.Context, cfg RunnerConfig, task ai.Task, scenarios []Scenario, repeats int,
	trace *payloadTrace, journal *runJournal, c candidate, log *slog.Logger,
) ([]Record, error) {
	log.InfoContext(ctx, "aicert: certifying", "task", string(task), "tier", string(c.Tier), "rung", c.Rung,
		"model", c.Binding.Model, "judge", c.Judge.Model)
	hooks := &certifyHooks{trace: trace, journal: journal.forTask(task, c.Binding, c.Judge)}
	rec, err := certifyTask(ctx, task, scenarios, cfg.Census, c.Binding, c.Judge, cfg.recordProfile(), repeats, log, hooks)
	if err != nil {
		log.ErrorContext(ctx, "aicert: task certification failed — no record written", "task", string(task), "model", c.Binding.Model, "err", err)
		return nil, fmt.Errorf("task %s on %s: %w", task, c.Binding.Model, err)
	}
	if err := WriteRecord(cfg.RecordDir, rec); err != nil {
		return nil, fmt.Errorf("task %s on %s: writing record: %w", task, c.Binding.Model, err)
	}
	if c.Rung != 0 {
		return []Record{rec}, nil
	}
	decisions, err := certifyDecisionsFor(ctx, cfg, task, scenarios, c.Binding, rec, hooks, log)
	if err != nil {
		log.ErrorContext(ctx, "aicert: decision leg failed — no decision record written", "task", string(task), "err", err)
		return []Record{rec}, fmt.Errorf("task %s: %w", task, err)
	}
	return append([]Record{rec}, decisions...), nil
}

// fallbacksNotMeasured logs, before anything is spent, every fallback the judge
// cannot grade, and returns them for the run's closing line: in a long run the
// skip must be readable at either end of the log.
func fallbacksNotMeasured(ctx context.Context, cfg RunnerConfig, tasks []ai.Task, log *slog.Logger) []string {
	var names []string
	for _, task := range tasks {
		_, skipped, err := taskCandidates(cfg, task)
		if err != nil {
			continue // certifyAndWrite reports it, per task
		}
		for _, s := range skipped {
			log.WarnContext(ctx, "aicert: skipped fallback", "task", string(s.Task), "model", s.Model, "reason", s.Reason)
			names = append(names, fmt.Sprintf("%s %s (%s)", s.Task, s.Model, s.Reason))
		}
	}
	return names
}
