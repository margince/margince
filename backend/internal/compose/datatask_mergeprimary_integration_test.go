// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mergeTestCompany(t *testing.T, e *integration.Env, name string) ids.CompanyID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func mergeTestContact(t *testing.T, e *integration.Env, name string) ids.ContactID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

func mergeTestEmploy(t *testing.T, e *integration.Env, contact ids.ContactID, company ids.CompanyID, primary bool) ids.UUID {
	t.Helper()
	edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &company, IsCurrentPrimary: &primary, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return edge.ID
}

// dropHeirFlag clears the flag the merge handed to the heir, without an audit
// row. That is the state a merge left before it handed the slot on, and no
// writer produces it any more.
func dropHeirFlag(t *testing.T, e *integration.Env, heir ids.UUID) {
	t.Helper()
	e.WsExec(t, `UPDATE relationship SET is_current_primary = false WHERE id = $1`, heir)
}

// companyMergeDroppedPrimary is a contact whose primary employer was merged
// into a company it already worked at, left with no primary. It returns the
// contact's edge at the kept company.
func companyMergeDroppedPrimary(t *testing.T, e *integration.Env, name string) ids.UUID {
	t.Helper()
	contact := mergeTestContact(t, e, name)
	retired, kept := mergeTestCompany(t, e, name+" Old Works"), mergeTestCompany(t, e, name+" New Works")
	mergeTestEmploy(t, e, contact, retired, true)
	heir := mergeTestEmploy(t, e, contact, kept, false)
	if _, err := e.Contacts.MergeCompany(e.Admin(), retired, kept, nil); err != nil {
		t.Fatal(err)
	}
	dropHeirFlag(t, e, heir)
	return heir
}

func isPrimary(t *testing.T, e *integration.Env, edge ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND is_current_primary`, edge) == 1
}

func TestTheMergePrimaryTaskRestoresBothMergesAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	companyHeir := companyMergeDroppedPrimary(t, e, "Ines Varga")
	survivor, merged := mergeTestContact(t, e, "Olek Brandt"), mergeTestContact(t, e, "Olek Brandt")
	employer := mergeTestCompany(t, e, "Brandt Shared Works")
	contactHeir := mergeTestEmploy(t, e, survivor, employer, false)
	mergeTestEmploy(t, e, merged, employer, true)
	if _, err := e.Contacts.MergeContact(e.Admin(), merged, survivor, nil); err != nil {
		t.Fatal(err)
	}
	dropHeirFlag(t, e, contactHeir)
	task := mergeDroppedPrimaryDataTask()
	ctx := datatask.Acting(e.Admin(), task.ID)

	runMachineAddressTask(ctx, t, e, task)

	for name, heir := range map[string]ids.UUID{"company merge": companyHeir, "contact merge": contactHeir} {
		if !isPrimary(t, e, heir) {
			t.Errorf("after a %s the kept edge is still not the primary employer", name)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship' AND entity_id = $1
			AND action = 'update' AND evidence ->> 'reason' = 'merge_dropped_primary' AND evidence ? 'data_task_run'`, heir); n != 1 {
			t.Errorf("after a %s the restored edge has %d repair audit rows, want 1", name, n)
		}
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("replay changed %d contacts", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log`); again != audits {
		t.Fatal("replay wrote more audit rows")
	}
}

func TestTheMergePrimaryTaskLeavesAFlagClearedByHandAfterTheMerge(t *testing.T) {
	e := integration.Setup(t)
	contact := mergeTestContact(t, e, "Pia Lemberg")
	retired, kept := mergeTestCompany(t, e, "Lemberg Old"), mergeTestCompany(t, e, "Lemberg New")
	mergeTestEmploy(t, e, contact, retired, true)
	heir := mergeTestEmploy(t, e, contact, kept, false)
	if _, err := e.Contacts.MergeCompany(e.Admin(), retired, kept, nil); err != nil {
		t.Fatal(err)
	}
	no := false
	if _, err := e.Contacts.UpdateRelationship(e.Admin(), heir, contacts.UpdateRelationshipInput{IsCurrentPrimary: &no}); err != nil {
		t.Fatal(err)
	}
	task := mergeDroppedPrimaryDataTask()

	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)

	if isPrimary(t, e, heir) {
		t.Fatal("the repair overruled a flag a person cleared after the merge")
	}
}

func TestTheMergePrimaryTaskLeavesAContactWithTwoCurrentEmployments(t *testing.T) {
	e := integration.Setup(t)
	contact := mergeTestContact(t, e, "Rune Sandvik")
	retired, kept, other := mergeTestCompany(t, e, "Sandvik Old"), mergeTestCompany(t, e, "Sandvik New"), mergeTestCompany(t, e, "Sandvik Side")
	mergeTestEmploy(t, e, contact, retired, true)
	heir := mergeTestEmploy(t, e, contact, kept, false)
	side := mergeTestEmploy(t, e, contact, other, false)
	if _, err := e.Contacts.MergeCompany(e.Admin(), retired, kept, nil); err != nil {
		t.Fatal(err)
	}
	dropHeirFlag(t, e, heir)
	task := mergeDroppedPrimaryDataTask()

	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)

	if isPrimary(t, e, heir) || isPrimary(t, e, side) {
		t.Fatal("the repair chose between two current employments")
	}
}

func TestTheMergePrimaryTaskBoundsPagingAndRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	first := companyMergeDroppedPrimary(t, e, "Saskia Ewald")
	second := companyMergeDroppedPrimary(t, e, "Timo Rautio")
	task := mergeDroppedPrimaryDataTask()
	ctx := datatask.Acting(e.Admin(), task.ID)
	count, more, err := task.Steps[0].Estimate(ctx, e.Pool, 1)
	if err != nil || count != 1 || !more {
		t.Fatalf("estimate %d, %v, %v", count, more, err)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		page, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err == nil && (page.Done || len(page.Cursor) == 0 || page.Changed != 1) {
			t.Errorf("first page: %+v", page)
		}
		return errors.New("interrupted after the first page")
	}); err == nil {
		t.Fatal("the interrupted transaction committed")
	}
	if isPrimary(t, e, first) || isPrimary(t, e, second) {
		t.Fatal("a restored employer escaped the rollback")
	}

	failing := task
	original := task.Steps[0].Batch
	failing.Steps = []datatask.Step{task.Steps[0]}
	failing.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupted after employer writes")
	}
	runMachineAddressTask(ctx, t, e, failing)
	if isPrimary(t, e, first) || isPrimary(t, e, second) {
		t.Fatal("a restored employer escaped the engine's rollback")
	}
}
