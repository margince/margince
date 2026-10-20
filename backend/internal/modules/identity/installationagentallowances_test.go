// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func filledAgentAllowancePatch() AgentAllowancePatch {
	value := func(n int) *int { return &n }
	return AgentAllowancePatch{
		WritesPerWindow: value(2), EgressPerWindow: value(3), CallsPerWindow: value(4),
		WindowHours: value(5),
	}
}

func TestEveryAgentAllowanceCountWritesItsOwnSetting(t *testing.T) {
	writes, err := filledAgentAllowancePatch().writes()
	if err != nil {
		t.Fatalf("encoding a full patch: %v", err)
	}
	written := map[string]string{}
	for _, write := range writes {
		written[write.key] = string(write.raw)
	}
	for key, want := range map[string]int{
		AgentWritesPerWindow.Key(): 2,
		AgentEgressPerWindow.Key(): 3, AgentCallsPerWindow.Key(): 4,
	} {
		if got := written[key]; got != strconv.Itoa(want) {
			t.Errorf("%s was written %q, want %d", key, got, want)
		}
	}
	if raw, ok := written[AgentWindowHours.Key()]; ok {
		t.Errorf("the window length was written straight (%s); it must wait for the next reset", raw)
	}
}

func TestAnEmptyAgentAllowancePatchWritesNothing(t *testing.T) {
	writes, err := AgentAllowancePatch{}.writes()
	if err != nil {
		t.Fatalf("encoding an empty patch: %v", err)
	}
	for _, write := range writes {
		if write.raw != nil {
			t.Errorf("%s encoded %s from an absent field", write.key, write.raw)
		}
	}
}

// A window length a boot source sets has no change waiting. A length an admin
// saved earlier would otherwise take over from it at the next reset.
func TestABootSetWindowLengthSetsAsideAWaitingChange(t *testing.T) {
	waiting := allowanceValues{200, 20, 1000, 24, 12, 1_800_000_000}
	if got := waiting.policy(nil); got.Window.Next != 12*time.Hour {
		t.Fatalf("with nothing set at boot the waiting change is %s, want 12h", got.Window.Next)
	}
	locked := environConfig(t, "MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS=24")
	if got := waiting.policy(locked); got.Window.Next != 0 || got.Window.Length != 24*time.Hour {
		t.Fatalf("under a boot-set length the plan is %+v, want 24h and nothing waiting", got.Window)
	}
}

// Under a boot-set length a request for another length is refused, and the
// same length is a no-op, before anything reaches the database.
func TestAWindowChangeUnderABootSetLengthIsAConflict(t *testing.T) {
	store := &InstallationSettingsStore{config: environConfig(t, "MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS=24")}
	admin := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{installationSettingsObject: {Read: true, Update: true}},
		},
	})
	other, same := 12, 24
	if err := store.scheduleAgentWindowTx(admin, nil, &other); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a different length = %v, want a conflict", err)
	}
	if err := store.scheduleAgentWindowTx(admin, nil, &same); err != nil {
		t.Fatalf("the boot-set length itself = %v, want a no-op", err)
	}
}
