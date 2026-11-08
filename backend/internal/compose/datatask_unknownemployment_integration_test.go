// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func unknownEmploymentLink(ctx context.Context, t *testing.T, e *integration.Env, name string, company ids.UUID) ids.UUID {
	t.Helper()
	contact := ids.From[ids.ContactKind](e.SeedContact(t, name, nil))
	employer := ids.From[ids.CompanyKind](company)
	row, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &employer, EmploymentStatus: new("unknown"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(row.ID)
}

func TestUnknownEmploymentLinksBecomeCurrentThroughTheExecutor(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Roster repair", nil)
	first := unknownEmploymentLink(e.Admin(), t, e, "First colleague", company)
	second := unknownEmploymentLink(e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms), t, e, "Second colleague", company)
	task := unknownEmploymentDataTask(e.Pool)
	task.Steps[0].Limit = 1
	ctx := datatask.Acting(e.Admin(), task.ID)
	runMachineAddressTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = ANY($1) AND employment_status = 'current'`, []ids.UUID{first, second}); got != 2 {
		t.Fatalf("corrected %d links", got)
	}
	row, err := e.Contacts.GetCompany(e.Admin(), ids.From[ids.CompanyKind](company), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.ContactCount == nil || *row.ContactCount != 2 {
		t.Fatalf("count = %v", row.ContactCount)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = ANY($1) AND evidence ? 'data_task_run'`, []ids.UUID{first, second}); got != 2 {
		t.Fatalf("repair audit evidence on %d links", got)
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if out.Changed != 0 {
			t.Errorf("nil-cursor replay changed %d", out.Changed)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log`); got != before {
		t.Fatal("replay added audits")
	}
}

func TestUnknownEmploymentRepairRollsBackWithProgress(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Rollback roster", nil)
	id := unknownEmploymentLink(e.Admin(), t, e, "Colleague", company)
	task := unknownEmploymentDataTask(e.Pool)
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupted after relationship writes")
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id=$1 AND employment_status='unknown'`, id); got != 1 {
		t.Fatal("relationship escaped rollback")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log`); got != before {
		t.Fatal("audit escaped rollback")
	}
}

func TestUnknownEmploymentRepairRejectsMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	task := unknownEmploymentDataTask(e.Pool)
	ctx := datatask.Acting(e.Admin(), task.ID)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`{"after":"not-an-id"}`), Limit: 1, RunID: ids.NewV7()})
		return err
	})
	if err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestUnknownEmploymentRepairUsesDatesAndSkipsArchivedEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		endYear                        int
		archiveContact, archiveCompany bool
		want                           string
	}{
		{name: "past end", endYear: 1970, want: "former"},
		{name: "future end", endYear: 9998, want: "current"},
		{name: "archived contact", archiveContact: true, want: "unknown"},
		{name: "archived company", archiveCompany: true, want: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			company := e.SeedCompany(t, "Employment dates", nil)
			id := unknownEmploymentLink(e.Admin(), t, e, "Colleague", company)
			row, err := e.Contacts.GetRelationship(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.endYear != 0 {
				end := time.Date(tc.endYear, 1, 1, 0, 0, 0, 0, time.UTC)
				if _, err := e.Contacts.UpdateRelationship(e.Admin(), id, contacts.UpdateRelationshipInput{EndedAt: &end}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.archiveContact {
				if _, err := e.Contacts.ArchiveContact(e.Admin(), *row.ContactID, nil); err != nil {
					t.Fatal(err)
				}
			}
			if tc.archiveCompany {
				if _, err := e.Contacts.ArchiveCompany(e.Admin(), ids.From[ids.CompanyKind](company), nil); err != nil {
					t.Fatal(err)
				}
			}
			task := unknownEmploymentDataTask(e.Pool)
			runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
			if e.WsCount(t, `SELECT count(*) FROM relationship WHERE id=$1 AND employment_status=$2`, id, tc.want) != 1 {
				t.Fatalf("expected status %s", tc.want)
			}
		})
	}
}
