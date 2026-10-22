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

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestTheBacklogGaugesRenderTheCountAndTheAgeInSeconds(t *testing.T) {
	var out bytes.Buffer
	err := writeThreadBacklog(&out, capture.ThreadBacklog{Waiting: 7, OldestWait: 12*time.Minute + 400*time.Millisecond})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	for _, want := range []string{
		"# TYPE margince_capture_confidentiality_waiting_threads gauge\n",
		"margince_capture_confidentiality_waiting_threads 7\n",
		"# TYPE margince_capture_confidentiality_oldest_wait_seconds gauge\n",
		"margince_capture_confidentiality_oldest_wait_seconds 720\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the exposition lacks %q:\n%s", want, out.String())
		}
	}
}

// Nothing waiting is a sample that reads 0, never a missing series: an alert
// on the age must see a caught-up pass as a number.
func TestAnEmptyBacklogRendersMeasuredZeros(t *testing.T) {
	var out bytes.Buffer
	if err := writeThreadBacklog(&out, capture.ThreadBacklog{}); err != nil {
		t.Fatalf("rendering: %v", err)
	}
	for _, want := range []string{
		"margince_capture_confidentiality_waiting_threads 0\n",
		"margince_capture_confidentiality_oldest_wait_seconds 0\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the exposition lacks %q:\n%s", want, out.String())
		}
	}
}

// A scrape that closes part way surfaces the fault wherever it falls, rather
// than reporting a sample it did not deliver. The writer is the one the job
// gauges' sweep uses.
func TestAClosedScrapeFailsTheBacklogWriteAtEachSample(t *testing.T) {
	for failOn := range 4 {
		w := &failingWriter{failOn: failOn, err: errors.New("the scrape connection closed")}
		if err := writeThreadBacklog(w, capture.ThreadBacklog{Waiting: 1}); err == nil {
			t.Errorf("a connection that refused write %d still reported success", failOn)
		}
	}
}

// A successful read renders both gauges through the section the exposition
// calls.
func TestTheBacklogSectionRendersWhatTheReaderReturns(t *testing.T) {
	var out bytes.Buffer
	section := threadBacklogSection(func(context.Context) (capture.ThreadBacklog, error) {
		return capture.ThreadBacklog{Waiting: 4, OldestWait: 90 * time.Second}, nil
	})
	if err := section(context.Background(), &out); err != nil {
		t.Fatalf("rendering the section: %v", err)
	}
	for _, want := range []string{
		"margince_capture_confidentiality_waiting_threads 4\n",
		"margince_capture_confidentiality_oldest_wait_seconds 90\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the section lacks %q:\n%s", want, out.String())
		}
	}
}

// A failed read writes nothing: a fabricated zero would read as a pass that has
// caught up.
func TestAFailedBacklogReadWritesNoSample(t *testing.T) {
	var out bytes.Buffer
	section := threadBacklogSection(func(context.Context) (capture.ThreadBacklog, error) {
		return capture.ThreadBacklog{}, errors.New("database unreachable")
	})
	if err := section(context.Background(), &out); err != nil {
		t.Fatalf("a failed read must not fail the scrape: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("a failed read wrote %q, want nothing", out.String())
	}
}
