// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contract on an archived anchor is not readable, whoever asks.
//
// A contract's visibility requires a LIVE anchor: an archived deal keeps its
// foreign key and its grants, so without the filter a contract stays readable
// through a record whose own read already answers 404.
//
// These pass on the tree before the change too, and that is the point of
// keeping them. The requirement travelled with the row-scope NARROWING, and the
// only thing making it unconditional was that `organization` is owner-private
// so its clause is never empty — an invariant holding on a neighbouring table's
// privacy setting. These cases assert the property directly, so it now has
// something of its own to stand on.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// anchorOnOrg puts the seeded deal on the organization the contract names: a
// contract validates that the two agree, and SeedDeal leaves the deal
// company-less.
func anchorOnOrg(t *testing.T, e *Env, deal, org ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// contractOnArchivedDeal plants an active contract and then archives the deal
// it hangs off, leaving the contract itself live.
func contractOnArchivedDeal(t *testing.T, e *Env) ids.ContractID {
	t.Helper()
	org := e.SeedOrg(t, "Acme", nil)
	pipeline, open, _ := DealFixture(t, e)
	dealUUID := e.SeedDeal(t, "Anchor", pipeline, open, &e.Rep1)
	deal := ids.From[ids.DealKind](dealUUID)
	anchorOnOrg(t, e, dealUUID, org)

	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		OrganizationID: ids.From[ids.OrganizationKind](org),
		DealID:         &deal,
		Title:          "An agreement on a deal that goes away",
		ValueBasis:     contracts.BasisTotal,
		Source:         "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE deal SET archived_at = now() WHERE id = $1`, dealUUID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ContractKind](ids.UUID(created.Id))
}

// The admin is the caller the old clause exempted: unbounded row scope, so
// nothing to narrow, so no clause — and the liveness filter disappeared with it.
func TestAnUnboundedCallerCannotReadAContractOnAnArchivedAnchor(t *testing.T) {
	e := Setup(t)
	id := contractOnArchivedDeal(t, e)

	_, err := e.Contracts.GetContract(e.Admin(), id)

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a contract whose anchor is archived answered %v, want not-found — a "+
			"live anchor is a property of the record, not a privilege a caller can out-rank", err)
	}
}

// The ordinary case still works: a live anchor is readable, so the change is a
// narrowing of exactly one shape rather than of the surface.
func TestAContractOnALiveAnchorStaysReadable(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", nil)
	pipeline, open, _ := DealFixture(t, e)
	dealUUID := e.SeedDeal(t, "Anchor", pipeline, open, &e.Rep1)
	deal := ids.From[ids.DealKind](dealUUID)
	anchorOnOrg(t, e, dealUUID, org)

	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		OrganizationID: ids.From[ids.OrganizationKind](org),
		DealID:         &deal,
		Title:          "An agreement on a live deal",
		ValueBasis:     contracts.BasisTotal,
		Source:         "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contracts.GetContract(e.Admin(),
		ids.From[ids.ContractKind](ids.UUID(created.Id))); err != nil {
		t.Fatalf("a contract on a live anchor stopped being readable: %v", err)
	}
}
