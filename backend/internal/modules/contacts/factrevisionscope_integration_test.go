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
	visibleOwner := ids.From[ids.UserKind](e.teammate)
	writable, err := e.store.CreateCompany(e.as(e.teammate, principal.RowScopeOwn), CreateCompanyInput{
		DisplayName: "Writable company", OwnerID: &visibleOwner, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	visible := ids.From[ids.CompanyKind](ids.UUID(writable.Id))
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
	owner := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `INSERT INTO record_grant
			(record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('company', $1, 'user', $2, 'read', $3)`, hidden, e.teammate, e.owner)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]ids.CompanyID{
		"read share on the survivor": {visible, hidden},
		"read share on the retired":  {hidden, visible},
	} {
		t.Run(name, func(t *testing.T) {
			err := e.store.tx(outsider, func(tx pgx.Tx) error {
				return settleFactRevisions(outsider, tx, pair[0], pair[1])
			})
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("settling with a read share answered %v, want permission denied", err)
			}
			if got := revisionOf(hidden); got != 7 {
				t.Fatalf("the read-shared company's counter moved to %d", got)
			}
			if got := revisionOf(visible); got != 3 {
				t.Fatalf("the visible company's counter moved to %d on a refused settle", got)
			}
		})
	}
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `UPDATE record_grant SET access = 'write'
			WHERE record_type = 'company' AND record_id = $1 AND subject_id = $2`, hidden, e.teammate)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(outsider, func(tx pgx.Tx) error {
		return settleFactRevisions(outsider, tx, visible, hidden)
	}); err != nil {
		t.Fatalf("a write share must permit settling the counters: %v", err)
	}
	if got := revisionOf(hidden); got != 8 {
		t.Fatalf("the write-shared survivor's counter is %d, want 8", got)
	}
	var retiredCounters int
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		return tx.QueryRow(owner, `SELECT count(*) FROM company_fact_revision WHERE company_id = $1`, visible).Scan(&retiredCounters)
	}); err != nil {
		t.Fatal(err)
	}
	if retiredCounters != 0 {
		t.Fatalf("the retired company kept %d fact counters", retiredCounters)
	}
}
