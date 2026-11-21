// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package outboxtest reads the envelope a fake transaction saw an outbox
// insert bind. It finds the envelope by what it is, not by its position, so a
// column the insert gains later breaks no test that reads it.
package outboxtest

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// Envelope is the one marshalled envelope among the binds of an outbox insert.
func Envelope(t testing.TB, binds []any) events.Envelope {
	t.Helper()
	var found []events.Envelope
	for _, bind := range binds {
		body, isBytes := bind.([]byte)
		if !isBytes {
			continue
		}
		var env events.Envelope
		if json.Unmarshal(body, &env) == nil && !env.EventID.IsZero() {
			found = append(found, env)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the outbox insert bound %d envelopes, want 1: %v", len(found), binds)
	}
	return found[0]
}
