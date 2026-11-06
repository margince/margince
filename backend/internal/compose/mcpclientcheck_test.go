// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheClientCheckGateReadsTheSwitchOnlyForAnAdmin(t *testing.T) {
	reads := 0
	gate := adminsWhileOn(func(context.Context) (bool, error) { reads++; return true, nil }, slog.New(slog.DiscardHandler))

	rep := principal.WithActor(context.Background(), principal.Principal{ID: "agent:rep", Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: []string{"rep"}}})
	if gate(rep) || reads != 0 {
		t.Fatalf("a non-admin was offered the tool (%v) or the switch was read %d times, want neither", gate(rep), reads)
	}
	admin := principal.WithActor(context.Background(), principal.Principal{ID: "agent:admin", Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: []string{"admin"}}})
	if !gate(admin) || reads != 1 {
		t.Errorf("an admin with the switch on was not offered the tool after one read (reads = %d)", reads)
	}
}

func TestTheClientCheckGateWithholdsTheToolWhenTheSwitchCannotBeRead(t *testing.T) {
	gate := adminsWhileOn(func(context.Context) (bool, error) { return true, errors.New("db down") }, slog.New(slog.DiscardHandler))
	admin := principal.WithActor(context.Background(), principal.Principal{ID: "agent:admin", Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: []string{"admin"}}})
	if gate(admin) {
		t.Error("an unreadable switch offered the tool")
	}
}

// ProtocolOnly skips both REST parity sweeps. So each tool that carries it is
// named here, and a new one is argued for in review.
func TestOnlyTheClientCheckSkipsTheRESTParitySweeps(t *testing.T) {
	var protocolOnly []string
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if spec.ProtocolOnly {
			protocolOnly = append(protocolOnly, spec.Name)
		}
	}
	if !slices.Equal(protocolOnly, []string{"check_client"}) {
		t.Errorf("the ProtocolOnly tools are %v, want only check_client", protocolOnly)
	}
}
