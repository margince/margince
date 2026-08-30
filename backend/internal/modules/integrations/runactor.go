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
// `provider_run` and `person_provider_claim` each carried a CHECK pinning them
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
// the provenance people.WriteProviderClaims writes onto the claim rows. One act
// leaves two rows, and a reader joining them has to see one name.
const connectorActorPrefix = "connector:"

// actingForProvider narrows the acting principal to the connector this run
// belongs to, and stamps a correlation id if the caller has not.
//
// It REPLACES whatever the worker bound. The worker's principal exists so an
// RBAC-gated write is never attempted with no actor at all; it cannot name a
// vendor, and this is the first point in the run where one is known.
func actingForProvider(ctx context.Context, name string) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: connectorActorPrefix + name,
	})
	if _, stamped := principal.CorrelationID(ctx); stamped {
		return ctx
	}
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
