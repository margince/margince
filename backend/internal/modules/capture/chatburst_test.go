// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A mail root is the sender's text, so the sink refuses the prefix a burst's
// ledger key starts with.
func TestARecordRootedAtABurstKeyIsRefused(t *testing.T) {
	rec := connector.NormalizedRecord{ThreadKey: threadverdict.BurstKey("0197c0aa-0000-7000-8000-000000000001")}
	if err := admitIndexableKeys(rec); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("a record under a burst key was admitted: %v", err)
	}
	rec.ThreadKey = "telegram:biz:880001:990100"
	if err := admitIndexableKeys(rec); err != nil {
		t.Fatalf("a chat thread key was refused: %v", err)
	}
}
