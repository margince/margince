// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func assertThreadRepairHealth(t *testing.T, e *integration.Env, want int) {
	t.Helper()
	source, findings, err := (Server{captureHealthHandlers: captureHealthHandlers{pool: e.Pool}}).captureSummary(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.Code != "capture_repair_unverified" {
			continue
		}
		if want == 0 || finding.Count == nil || *finding.Count != want || string(finding.Condition) != string(healthTechnical) || finding.Responsible != "operator" {
			t.Fatalf("quarantined repair finding=%+v, want %d technical operator runs", finding, want)
		}
		if source.Condition != healthTechnical || source.Code != finding.Code {
			t.Fatalf("quarantined repair summary=%+v", source)
		}
		return
	}
	if want != 0 {
		t.Fatalf("missing quarantined repair finding: %+v", findings)
	}
}
