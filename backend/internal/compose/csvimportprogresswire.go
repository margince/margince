// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a run says about its work in flight, computed when it is read. Every age
// is taken against the database clock of that read. A wrong clock in a browser
// or in the API host cannot make a run look stalled.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// applyProgress copies the phase, the counts, the clock and the speed onto the
// contract run.
func applyProgress(out *crmcontracts.ImportRun, run migration.Run) {
	if run.Phase != "" {
		out.Phase = new(crmcontracts.ImportRunPhase(run.Phase))
	}
	if run.PhaseObject != "" {
		out.PhaseObject = &run.PhaseObject
	}
	if run.Progress != nil {
		out.Progress = progressOf(*run.Progress)
	}
	out.StartedAt = run.StartedAt
	out.FinishedAt = finishedAtOf(run)
	if run.StartedAt != nil {
		end := run.ReadAt
		if out.FinishedAt != nil {
			end = *out.FinishedAt
		}
		out.ElapsedSeconds = secondsBetween(*run.StartedAt, end)
	}
	if !working(run.Status) {
		return
	}
	out.IdleSeconds = ageOf(run.ProgressAt, run.ReadAt)
	out.HeartbeatAgeSeconds = ageOf(run.HeartbeatAt, run.ReadAt)
	if run.Progress == nil || run.ProgressAt == nil {
		return
	}
	if est, ok := migration.EstimateOf(*run.Progress, *run.ProgressAt, run.ReadAt); ok {
		rate := float32(est.RowsPerSecond)
		scope := crmcontracts.ImportRunEtaScope(est.Scope)
		out.RateRowsPerSecond, out.EtaSecondsLow, out.EtaSecondsHigh, out.EtaScope = &rate, &est.LowSeconds, &est.HighSeconds, &scope
	}
}

// working is whether a worker holds the run: only then is an age or a speed a
// fact about something happening.
func working(status string) bool {
	return status == migration.StatusValidating || status == migration.StatusRunning || status == migration.StatusUndoing
}

// finishedAtOf is when a run reached a final status. A run that ended without
// the column set, such as a cancel, reads its last change.
func finishedAtOf(run migration.Run) *time.Time {
	switch run.Status {
	case migration.StatusComplete, migration.StatusUndone, migration.StatusCancelled:
	default:
		return nil
	}
	if run.FinishedAt != nil {
		return run.FinishedAt
	}
	updated := run.UpdatedAt
	return &updated
}

func secondsBetween(from, to time.Time) *int {
	seconds := max(0, int(to.Sub(from).Seconds()))
	return &seconds
}

func ageOf(at *time.Time, readAt time.Time) *int {
	if at == nil {
		return nil
	}
	return secondsBetween(*at, readAt)
}

func progressOf(p migration.Progress) *crmcontracts.ImportRunProgress {
	out := crmcontracts.ImportRunProgress{
		Objects: make([]struct {
			Object    string `json:"object"`
			RowsDone  int    `json:"rows_done"`
			RowsTotal int    `json:"rows_total"`
		}, 0, len(p.Objects)),
	}
	for _, o := range p.Objects {
		out.Objects = append(out.Objects, struct {
			Object    string `json:"object"`
			RowsDone  int    `json:"rows_done"`
			RowsTotal int    `json:"rows_total"`
		}{Object: o.Object, RowsDone: o.RowsDone, RowsTotal: o.RowsTotal})
	}
	out.Counts.Created, out.Counts.Updated, out.Counts.Unchanged = p.Counts.Created, p.Counts.Updated, p.Counts.Unchanged
	out.Counts.Skipped, out.Counts.Problems = p.Counts.Skipped, p.Counts.Problems
	return &out
}

// reasonCodeOrAbsent is a report row's code, or nil for a row stored without one.
func reasonCodeOrAbsent(code migration.ReasonCode) *crmcontracts.ImportReasonCode {
	if code == "" {
		return nil
	}
	return new(crmcontracts.ImportReasonCode(code))
}

func paramsOrAbsent(params migration.Params) *map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := map[string]string(params)
	return &out
}
