// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
)

// importSample reads one series' current value off the process exposition;
// the collector is process-wide, so assertions are deltas.
func importSample(t *testing.T, series string) float64 {
	t.Helper()
	var b strings.Builder
	capturemetrics.WriteProcessMetrics(&b)
	for line := range strings.SplitSeq(b.String(), "\n") {
		if value, ok := strings.CutPrefix(line, series+" "); ok {
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				t.Fatalf("series %s carries an unparseable value %q", series, value)
			}
			return f
		}
	}
	return 0
}

// upsertAsBackfill lands one record the way a backfill walk does: under a run
// that names its provider, inside one message's tally, with a skip tallied as
// an uncaptured message rather than a failure.
func upsertAsBackfill(ctx context.Context, sink *capture.Sink, sourceID, counterparty string, addresses ...string) error {
	ctx, message := capturemetrics.BeginMessage(capturemetrics.ForProvider(ctx, "imap"))
	_, err := sink.Upsert(ctx, mailRecord(sourceID, counterparty, addresses...))
	if isSkip(err) {
		message.End(false, nil)
	} else {
		message.End(err == nil, err)
	}
	return err
}

// Inside a backfill the sink times its transaction and the work after it, and
// the trace's decision becomes the message's outcome: an all-internal message
// is counted internal, not as a generic skip.
func TestABackfilledMessageIsTimedBySinkStageAndCountedByItsTracedDecision(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	sink := capture.NewSink(db)
	series := []string{
		`margince_capture_backfill_stage_seconds_count{provider="imap",stage="sink"}`,
		`margince_capture_backfill_stage_seconds_count{provider="imap",stage="ensure"}`,
		`margince_capture_backfill_messages_total{provider="imap",outcome="internal"}`,
	}
	before := map[string]float64{}
	for _, s := range series {
		before[s] = importSample(t, s)
	}

	if err := upsertAsBackfill(ctx, sink, "metrics-internal-1", "boss@acme.com", "boss@acme.com", "rep@acme.com"); !isSkip(err) {
		t.Fatalf("an all-internal message: got %v, want a skip", err)
	}
	if err := upsertAsBackfill(ctx, sink, "metrics-external-1", "buyer@customer.example", "buyer@customer.example", "rep@acme.com"); err != nil {
		t.Fatalf("an external message: %v", err)
	}

	for s, want := range map[string]float64{series[0]: 2, series[1]: 1, series[2]: 1} {
		if moved := importSample(t, s) - before[s]; moved != want {
			t.Errorf("%s moved by %v, want %v", s, moved, want)
		}
	}
}
