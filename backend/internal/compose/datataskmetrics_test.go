// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
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

func TestCorrectionMetricsPropagateEveryFailedWrite(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	progress := []datatask.Progress{{TaskID: "core/100_probe", Step: "first", Status: datatask.Done, LastProgressAt: &now}}
	refused := errors.New("reader disconnected")
	for _, render := range []func(io.Writer) error{
		func(w io.Writer) error { return writeDataTaskRead(w, true) },
		func(w io.Writer) error { return writeDataTaskMetrics(w, progress, now) },
	} {
		succeeded := false
		for n := range 32 {
			writer := &failingWriter{failOn: n, err: refused}
			err := render(writer)
			if err == nil {
				if writer.writes > n {
					t.Fatalf("write %d was refused but the report succeeded", n)
				}
				succeeded = true
				break
			}
			if !errors.Is(err, refused) {
				t.Fatalf("write %d lost its cause: %v", n, err)
			}
		}
		if !succeeded {
			t.Fatal("writer sweep did not reach the end of the report")
		}
	}
}
