// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat that may not see a company cannot move its fact counter through the
// merge's settle step, in either role. Both counters stay as they were.
func TestFactRevisionsSettleOnlyForCompaniesTheCallerSees(t *testing.T) {
	e := setupCapturePrivacy(t)
	hidden := e.captureCompany(t, "owner")
	visible := e.captureCompany(t, "workspace")
	seedRevision := func(company ids.CompanyID, revision int) {
		t.Helper()
		ctx := e.as(e.owner, principal.RowScopeOwn)
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO company_fact_revision (company_id, revision) VALUES ($1, $2)`, company, revision)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	revisionOf := func(company ids.CompanyID) int {
		t.Helper()
		ctx := e.as(e.owner, principal.RowScopeOwn)
		var got int
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT revision FROM company_fact_revision WHERE company_id = $1`, company).Scan(&got)
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	seedRevision(hidden, 7)
	seedRevision(visible, 3)
	outsider := e.as(e.teammate, principal.RowScopeOwn)
	for name, pair := range map[string][2]ids.CompanyID{
		"the hidden company as survivor": {visible, hidden},
		"the hidden company as retired":  {hidden, visible},
	} {
		t.Run(name, func(t *testing.T) {
			err := e.store.tx(outsider, func(tx pgx.Tx) error {
				return settleFactRevisions(outsider, tx, pair[0], pair[1])
			})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("settling with a company the seat cannot see answered %v, want not found", err)
			}
			if got := revisionOf(hidden); got != 7 {
				t.Fatalf("the hidden company's counter moved to %d", got)
			}
			if got := revisionOf(visible); got != 3 {
				t.Fatalf("the visible company's counter moved to %d on a refused settle", got)
			}
		})
	}
}
