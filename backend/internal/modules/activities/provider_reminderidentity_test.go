// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The reminder identity is reserved against clients and required by the engine,
// and this seam carries both. These tests pin the line between them to the
// PRINCIPAL — the one thing a request body cannot spell.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func reminderCreate(system string) crmcontracts.CreateActivityRequest {
	return crmcontracts.CreateActivityRequest{
		Kind: "task", SourceSystem: &system, SourceId: strPtr("no_activity_reminder:company:c-1:anchor:2026-09-05T00:00:00Z"),
	}
}

// Without this the reservation breaks the feature it exists to protect: the
// engine's own create runs through the same seam a tool call does, so refusing
// the identity outright would refuse every reminder the engine ever writes.
func TestTheEngineMayStampItsOwnReminderIdentity(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	for _, source := range []string{provenance.NoActivityReminderSource, provenance.CheckInCadenceSource} {
		in, err := logInputForPrincipal(ctx, reminderCreate(source))
		if err != nil {
			t.Fatalf("[%s] the engine could not stamp its own reminder identity: %v", source, err)
		}
		if in.SourceSystem == nil || *in.SourceSystem != source {
			t.Errorf("[%s] SourceSystem = %v, want it carried through", source, in.SourceSystem)
		}
	}
}

// The principal is the whole guard, so a caller who is not the system is
// refused the same identity the system just wrote.
func TestAnOrdinaryCallerMayNotStampAReminderIdentity(t *testing.T) {
	// The agent is the tool surface, which is the realistic way a reminder
	// identity would be spelled at this seam rather than at the HTTP wire.
	for _, ctx := range []context.Context{
		context.Background(),
		principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "u-1"}),
		principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: "a-1"}),
		principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalConnector, ID: "c-1"}),
	} {
		for _, source := range []string{provenance.NoActivityReminderSource, provenance.CheckInCadenceSource} {
			_, err := logInputForPrincipal(ctx, reminderCreate(source))
			var refused *provenance.ReservedError
			if !errors.As(err, &refused) {
				t.Fatalf("[%s] err = %v, want ReservedError — a caller able to write this identity can suppress the reminder", source, err)
			}
		}
	}
}

// The exception is the reminder identities and nothing else: the importer's
// namespace has its own writer, so the system principal buys no admission to it.
func TestTheSystemPrincipalDoesNotUnlockTheImporterNamespace(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	_, err := logInputForPrincipal(ctx, reminderCreate(provenance.ReservedSourceSystemPrefix+"legacy_crm"))
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want ReservedError — the import namespace is not the engine's to write", err)
	}
}
