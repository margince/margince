// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"os"
	"strings"
	"testing"
)

// TestASinkRefusesToAppendBehindAFailedWrite holds the one thing a JSONL reader
// cannot recover from.
//
// A failed Encode can leave a partial line on disk, and readLive stops at the
// first line it cannot decode. Appending past one would strand every LATER run
// behind it: the file keeps growing and keeps replaying nothing, which reads
// exactly like a journal that never had anything to replay — the silent
// failure this whole mechanism exists to avoid.
func TestASinkRefusesToAppendBehindAFailedWrite(t *testing.T) {
	sink, err := openJSONLSink(t.TempDir(), "lines.jsonl", "test sink", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		t.Fatalf("openJSONLSink: %v", err)
	}
	if err := encodeLine(sink, journaledRun{Task: "before", Run: 1}); err != nil {
		t.Fatalf("the first line must be writable: %v", err)
	}
	// Closing the file is how a write is made to fail without a fake writer.
	if err := sink.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := encodeLine(sink, journaledRun{Task: "fails", Run: 2}); err == nil {
		t.Fatal("a write to a closed sink must report the failure")
	}

	err = encodeLine(sink, journaledRun{Task: "after", Run: 3})
	if err == nil {
		t.Fatal("the sink kept appending after a failed write — a later reader stops at the damaged line and never reaches this one")
	}
	if !strings.Contains(err.Error(), "earlier failed write") {
		t.Fatalf("error %q does not say the sink was already broken", err)
	}
}
