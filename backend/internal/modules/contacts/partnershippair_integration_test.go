// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company is a partner when both halves exist: a partner row, and the
// partner relationship type on the company. Two tables hold one fact, so
// either half alone is a state the contract calls impossible.
//
// This asks the STATE rather than a code path, and it asks it of every row
// rather than of the company under test. A test that drove one endpoint would
// hold that endpoint; the next writer of either table would walk past it. The
// query here fails on whatever left a row behind.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assertPartnershipPaired fails naming every company whose two halves disagree,
// in both directions.
func assertPartnershipPaired(ctx context.Context, t *testing.T, e *dedupeEnv, after string) {
	t.Helper()
	var halves []string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.company_id::text || ' has a partner row and no live partner type'
			  FROM partner p
			 WHERE p.archived_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM company_relationship_type t
			                    WHERE t.company_id = p.company_id
			                      AND t.relationship_type = 'partner'
			                      AND t.archived_at IS NULL)
			UNION ALL
			SELECT t.company_id::text || ' carries the partner type and has no live partner row'
			  FROM company_relationship_type t
			 WHERE t.relationship_type = 'partner' AND t.archived_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM partner p
			                    WHERE p.company_id = t.company_id
			                      AND p.archived_at IS NULL)`)
		if err != nil {
			return err
		}
		halves, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading the partnership pair after %s: %v", after, err)
	}
	for _, half := range halves {
		t.Errorf("after %s: %s", after, half)
	}
}

// pairRestore is what this restore needs and no more. Neither the erasure
// boundary nor the link restorers are the subject here. The partner row and
// the relationship type come back through the un-archive's own statements,
// which run whatever those two answer.
var pairRestore = storekit.RestoreWith{
	Erased: func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return false, nil },
	Links: storekit.LinkRestore{
		Membership: func(context.Context, pgx.Tx, string, ids.UUID, storekit.ListMembership) (bool, error) {
			return false, nil
		},
		Tag: func(context.Context, pgx.Tx, string, ids.UUID, storekit.TagAssignment) (bool, error) {
			return false, nil
		},
	},
}

func TestThePartnershipPairIsNeverHalfWritten(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Pair Partner GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	assertPartnershipPaired(ctx, t, e, "a company with no programme")

	if _, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{
		CompanyID: companyID, PartnerRole: "hosting",
	}); err != nil {
		t.Fatalf("promote to partner: %v", err)
	}
	assertPartnershipPaired(ctx, t, e, "the promotion")

	// Archiving the company takes both halves, and restoring brings both back.
	// Either statement losing its partner would show here rather than in the
	// sweep that wrote it.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return e.store.ArchiveCompanyTx(ctx, tx, companyID, nil)
	}); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}
	assertPartnershipPaired(ctx, t, e, "archiving the company")

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.RestoreCompanyTx(ctx, tx, companyID, nil, pairRestore)
		return err
	}); err != nil {
		t.Fatalf("restoring the company: %v", err)
	}
	assertPartnershipPaired(ctx, t, e, "restoring the company")
}
