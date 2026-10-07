// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCaptureMetricsNamesEveryOutcomeItCounted(t *testing.T) {
	var buf bytes.Buffer
	writeCaptureMetrics(&buf, map[string]uint64{"captured": 12, "internal": 3})
	out := buf.String()

	for _, want := range []string{
		"# TYPE margince_capture_outcomes_total counter",
		`margince_capture_outcomes_total{outcome="captured"} 12`,
		`margince_capture_outcomes_total{outcome="internal"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q, got:\n%s", want, out)
		}
	}
	// Sorted so a human reading a scrape by hand sees a stable block.
	if strings.Index(out, `outcome="captured"`) > strings.Index(out, `outcome="internal"`) {
		t.Error("outcomes are not sorted")
	}
}

// A process that has traced nothing has not decided nothing — it has not run.
// Printing zeros would report the first as the second, so only the family's
// declaration is written.
func TestCaptureMetricsWritesNoSampleWhenNothingWasTraced(t *testing.T) {
	var buf bytes.Buffer
	writeCaptureMetrics(&buf, nil)
	if strings.Contains(buf.String(), "margince_capture_outcomes_total{") {
		t.Errorf("exposition = %q for an untraced process, want no sample", buf.String())
	}
	if !strings.Contains(buf.String(), "# TYPE margince_capture_outcomes_total counter\n") {
		t.Errorf("exposition = %q, want the family declared", buf.String())
	}
}

// A WARN line about a rate limit carries the limit Google named and the status
// it came on, inline; a line about any other fault carries neither.
func TestARateLimitLogLineCarriesTheLimitAndStatus(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	limited := &connector.RateLimitedError{Reason: "userRateLimitExceeded", Status: http.StatusForbidden}
	log.Warn("capture connection sync failed", "err", fmt.Errorf("sync: %w", limited), rateLimitAttr(fmt.Errorf("sync: %w", limited)))
	if !strings.Contains(buf.String(), " reason=userRateLimitExceeded status=403") {
		t.Errorf("log line = %q, want the reason and status inline", buf.String())
	}
	buf.Reset()
	log.Warn("capture connection sync failed", "err", errors.New("unreachable"), rateLimitAttr(errors.New("unreachable")))
	if strings.Contains(buf.String(), "reason=") || strings.Contains(buf.String(), "status=") {
		t.Errorf("log line = %q, want no rate-limit fields", buf.String())
	}
}
