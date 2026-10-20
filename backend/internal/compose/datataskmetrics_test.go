// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestCorrectionMetricsPreserveUnknownProgress(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	progress := []datatask.Progress{{TaskID: "core/100_known", Step: "first", Status: datatask.Running, LastProgressAt: &past}, {TaskID: "core/200_unknown", Step: "first", Status: datatask.Pending}}
	var out bytes.Buffer
	if err := writeDataTaskMetrics(&out, progress, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`margince_data_task_steps{status="running"} 1`, `margince_data_task_steps{status="pending"} 1`, `margince_data_task_seconds_since_progress{task="core/100_known",step="first"} 60`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), `task="core/200_unknown"`) {
		t.Fatal("invented a last-progress age")
	}
	out.Reset()
	if err := failedDataTaskRead(context.Background(), &out, errors.New("unreadable")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "margince_data_task_read_success 0") || strings.Contains(out.String(), "margince_data_task_steps{") {
		t.Fatal("failed read reported empty work")
	}
}
