// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs

// ClearPeriodicsForTest isolates manually queued work from unrelated scheduled writes.
func (r *Runner) ClearPeriodicsForTest() {
	//nolint:forbidigo // integration-only isolation; production schedules still come from the schedule book
	r.client.PeriodicJobs().Clear()
}
