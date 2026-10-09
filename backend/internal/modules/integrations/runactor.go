// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// Who a provider run acts as, derived from the run.
//
// The connector is the actor because every value these runs write is BOUGHT
// from the provider rather than typed by anybody: a reader of a record has to
// be able to tell purchased data from a colleague's entry, and the audit row is
// where they read it.
//
// WHICH connector is a fact about the run, and only this module can resolve it.
// The workers that execute a run know a run id — the poll sweep drains many at
// once — so a principal bound out there can only ever name a vendor it guessed.
// It guessed the one provider that could exist while `provider_connection`,
// `provider_run` and `contact_provider_claim` each carried a CHECK pinning them
// to a single name. Those checks are gone so a second provider can be
// connected, and a guess is now a wrong answer: the claim rows derive their
// provenance from the run's own provider, so the audit log and the evidence on
// the record would name different vendors for one purchase.
//
// An audit entry naming the wrong actor is worse than a missing one, because it
// reads as authoritative.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// connectorActorPrefix is what a connector's actor id is spelled with, matching
// the provenance contacts.WriteProviderClaims writes onto the claim rows. One act
// leaves two rows, and a reader joining them has to see one name.
const connectorActorPrefix = "connector:"

// actingForProvider narrows the acting principal to the connector this run
// belongs to, and stamps a correlation id if the caller has not.
//
// It REPLACES whatever the worker bound. The worker's principal exists so an
// RBAC-gated write is never attempted with no actor at all; it cannot name a
// vendor, and this is the first point in the run where one is known.
func actingForProvider(ctx context.Context, name string) context.Context {
	ctx = principal.WithActor(ctx, providerRunPrincipal(name))
	if _, stamped := principal.CorrelationID(ctx); stamped {
		return ctx
	}
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// providerRunPrincipal is who a run acts as.
//
// PrincipalConnector and not PrincipalSystem, so actor_type and actor_id
// agree. storekit stamps one from each. A system type beside a `connector:` id
// writes an audit row that contradicts itself.
//
// It carries permissions because that type costs it the system exemption.
// auth.Require admits a PrincipalSystem unconditionally, and every other type
// falls through to Permissions.Allows. A zero Permissions denies.
//
// Each grant answers a denial the run took:
//
//   - relationship and company read. The identifiers are resolved again at
//     submit, and a caller without the employer edge gets a name-only answer.
//     The run then skips as `no_identifiers`.
//   - contact read and update, for the fold through the row probe.
//   - RowScopeAll, because a run buys for the installation, not for a seat.
//     Capture privacy is lifted in platform/auth, since row scope alone does
//     not reach an owner-private row.
//
// A function rather than a literal so a test can assert the real thing.
func providerRunPrincipal(name string) principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   connectorActorPrefix + name,
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"},
			Objects: map[string]principal.ObjectGrant{
				"relationship": {Read: true},
				"company":      {Read: true},
				"contact":      {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	}
}
