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

// While the installation is locked no tool runs, a read included, and the call
// answers the lock's reason. The refusal comes before the tool is looked up, so
// nothing it would touch is reached.
func TestALockedInstallationRefusesEveryToolCall(t *testing.T) {
	for _, reason := range []error{apperrors.ErrTrialEnded, apperrors.ErrLicenseLapsed} {
		registry := NewRegistry(nil, nil, WithInstallationLock(func(context.Context) error { return reason }))
		registry.Register(plainTool{})

		for _, name := range []string{plainTool{}.Spec().Name, "no_such_tool"} {
			_, _, err := registry.InvokeServing(context.Background(), name, json.RawMessage(`{}`))
			if !errors.Is(err, reason) {
				t.Errorf("InvokeServing(%q) = %v, want %v", name, err, reason)
			}
		}
	}
}

// An installation that is not locked lets the call reach the tool lookup.
func TestAnUnlockedInstallationLetsToolCallsThrough(t *testing.T) {
	registry := NewRegistry(nil, nil, WithInstallationLock(func(context.Context) error { return nil }))
	_, _, err := registry.InvokeServing(context.Background(), "no_such_tool", json.RawMessage(`{}`))
	if unknown, ok := errors.AsType[*UnknownToolError](err); !ok || unknown.Name != "no_such_tool" {
		t.Errorf("InvokeServing = %v, want the unknown-tool refusal the lock let through", err)
	}
}
