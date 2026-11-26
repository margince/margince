// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import "github.com/margince/margince/backend/internal/platform/database"

func writeReadAdmissionMetrics(out *exposition, cost database.ReadAdmissionCost) {
	const gaugeKind = "gauge"
	for _, metric := range []struct {
		name, kind, help string
		value            float64
	}{
		{"attempts_total", "counter", "Expensive reads requesting capacity, including canceled waits.", float64(cost.Attempts)},
		{"wait_seconds_total", "counter", "Time spent waiting for expensive-read capacity.", cost.Wait.Seconds()},
		{"wait_seconds_max", gaugeKind, "Longest wait for expensive-read capacity since process start.", cost.MaxWait.Seconds()},
		{"waiters", gaugeKind, "Expensive reads waiting for capacity.", float64(cost.Waiters)},
		{"waiters_peak", gaugeKind, "Most concurrent capacity waiters since process start.", float64(cost.PeakWaiters)},
		{"active", gaugeKind, "Expensive reads holding capacity.", float64(cost.Active)},
	} {
		name := "margince_read_admission_" + metric.name
		out.printf("# HELP %s %s\n# TYPE %s %s\n%s %g\n", name, metric.help, name, metric.kind, name, metric.value)
	}
}
