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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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

// Through Provider.Create itself, not the helper: the helper being correct says
// nothing about the provider being wired to it, and the wiring is what a caller
// actually reaches. The store is nil because the refusal lands before any write
// — if this ever panics instead of refusing, the guard has moved behind the
// store call and a reserved identity is reaching the database.
func TestTheProviderRefusesAReminderIdentityFromAnOrdinaryCaller(t *testing.T) {
	provider := &Provider{}
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: "a-1"})
	for _, source := range []string{provenance.NoActivityReminderSource, provenance.CheckInCadenceSource} {
		_, err := provider.Create(ctx, datasource.CreateInput{
			EntityType: datasource.EntityActivity,
			Fields: map[string]any{
				"kind": "task", "subject": "planted",
				"source_system": source, "source_id": "planted-key",
			},
		})
		var refused *provenance.ReservedError
		if !errors.As(err, &refused) {
			t.Fatalf("[%s] Provider.Create err = %v, want ReservedError — the seam must reach the guard", source, err)
		}
	}
}

// The permissive branch carries the SAME source guard as the ordinary one: the
// engine's admission is for its reminder identity, and buys no licence to write
// a reserved `source` alongside it.
func TestTheEngineStillMayNotWriteAReservedSource(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	req := reminderCreate(provenance.NoActivityReminderSource)
	req.Source = provenance.ReservedSourceSystemPrefix + "legacy_crm:activity:a-1"
	_, err := logInputForPrincipal(ctx, req)
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want ReservedError — the import namespace is not the engine's to write on source either", err)
	}
	if refused.Field != "source" {
		t.Errorf("refusal names field %q, want source", refused.Field)
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

// HANDLERS ONLY, and this is where that is pinned.
//
// auth.DeclaredImporter is computed in the two HTTP handlers and nowhere else.
// The provider seam is the AGENT's door onto the same store, and an agent
// carries its granting human's whole Permissions — so a human holding
// import_run:create whose agent reached this seam would otherwise hand the
// agent the importer's namespace.
//
// The refusal here does not depend on the principal's grants at all, which is
// the point: this seam never asks, so there is nothing to grant.
func TestTheProviderSeamDoesNotOpenTheImporterNamespace(t *testing.T) {
	importer := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:admin",
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			"import_run": {Create: true},
		}},
	}
	ctx := principal.WithActor(context.Background(), importer)

	_, err := logInputForPrincipal(ctx, reminderCreate(provenance.ReservedSourceSystemPrefix+"hubspot"))
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want ReservedError — the importer's door is the HTTP handler, not this seam", err)
	}
	if refused.Field != "source_system" {
		t.Errorf("refusal names %q, want source_system", refused.Field)
	}
}
