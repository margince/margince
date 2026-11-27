// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The token correction refuses three callers before it reads. They are a
// non-system caller, an unreadable cursor and an estimate with no grant.
func TestTheTokenCorrectionRefusesBeforeItReads(t *testing.T) {
	if _, err := ZeroNegativeTokenCountsTx(context.Background(), nil, datatask.BatchIn{Limit: 1}); err == nil {
		t.Error("a batch outside the system = nil error, want a refusal")
	}
	system := principal.SystemActing(context.Background(), "system:test")
	if _, err := ZeroNegativeTokenCountsTx(system, nil, datatask.BatchIn{Limit: 1, Cursor: []byte("not a cursor")}); err == nil {
		t.Error("a batch with an unreadable cursor = nil error, want a refusal")
	}
	if _, _, err := CountNegativeTokenCalls(context.Background(), nil, 10); err == nil {
		t.Error("an estimate by a caller with no data task grant = nil error, want a refusal")
	}
}
