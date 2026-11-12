// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Once the trial has ended no tool runs, a read included. The refusal comes
// before the tool is looked up, so nothing it would touch is reached.
func TestAnEndedTrialRefusesEveryToolCall(t *testing.T) {
	registry := NewRegistry(nil, nil, WithTrialLock(func(context.Context) (bool, error) { return true, nil }))
	registry.Register(plainTool{})

	for _, name := range []string{plainTool{}.Spec().Name, "no_such_tool"} {
		_, _, err := registry.InvokeServing(context.Background(), name, json.RawMessage(`{}`))
		if !errors.Is(err, apperrors.ErrTrialEnded) {
			t.Errorf("InvokeServing(%q) = %v, want ErrTrialEnded", name, err)
		}
	}
}

// A trial still running locks nothing: the call reaches the tool lookup.
func TestARunningTrialLetsToolCallsThrough(t *testing.T) {
	registry := NewRegistry(nil, nil, WithTrialLock(func(context.Context) (bool, error) { return false, nil }))
	_, _, err := registry.InvokeServing(context.Background(), "no_such_tool", json.RawMessage(`{}`))
	if unknown, ok := errors.AsType[*UnknownToolError](err); !ok || unknown.Name != "no_such_tool" {
		t.Errorf("InvokeServing = %v, want the unknown-tool refusal the lock let through", err)
	}
}
