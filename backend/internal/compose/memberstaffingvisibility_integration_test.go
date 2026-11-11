// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMemberAdministrationCountsDoNotDiscloseUnreadableStaffing(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Hidden account", &e.Rep2)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Hidden opportunity", pipeline, stage, &e.Rep2)
	store := assignments.NewStore(e.DB())
	for _, parent := range []struct {
		kind crmcontracts.AssignmentRecordType
		id   ids.UUID
	}{
		{crmcontracts.AssignmentRecordTypeCompany, company},
		{crmcontracts.AssignmentRecordTypeDeal, deal},
	} {
		role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE active AND @kind=ANY(record_types) AND 'user'=ANY(assignee_kinds) ORDER BY key LIMIT 1`, pgx.StrictNamedArgs{"kind": string(parent.kind)}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateAssignment(e.Admin(), assignments.CreateAssignmentInput{RecordType: parent.kind, RecordID: parent.id, SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep1, RoleID: role}); err != nil {
			t.Fatal(err)
		}
	}
	permissions := principal.Permissions{Objects: map[string]principal.ObjectGrant{"user_admin": {Read: true, Update: true}}, RowScope: principal.RowScopeOwn}
	ctx := integration.OnSeat(e.As(e.AdminUser, nil, permissions), principal.SeatFull)
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		visible, err := assignments.OwnedMemberAssignmentsTx(ctx, tx, e.Rep1)
		if err != nil {
			return err
		}
		if visible != 0 {
			t.Fatalf("member administration disclosed %d unreadable responsibilities", visible)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx = integration.OnSeat(e.As(e.AdminUser, nil, permissions), principal.SeatFull)
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		snapshot, err := assignments.InspectMemberAssignmentsTx(ctx, tx, e.Rep1, nil)
		if err != nil {
			return err
		}
		if snapshot.Owned != 2 || snapshot.Visible != 0 || snapshot.Transferable != 0 {
			t.Fatalf("private dependency snapshot = %+v", snapshot)
		}
		stamp := strings.Join(snapshot.Stamp, ";")
		for _, secret := range []string{company.String(), deal.String(), "Hidden account", "Hidden opportunity"} {
			if strings.Contains(stamp, secret) {
				t.Fatal("dependency stamps disclose parent identity")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStaffingMetadataRequiresMemberAdministration(t *testing.T) {
	e := integration.Setup(t)
	for _, scenario := range []struct {
		name string
		run  func(context.Context, pgx.Tx) error
	}{
		{"visible counts", func(ctx context.Context, tx pgx.Tx) error {
			_, err := assignments.OwnedMemberAssignmentsTx(ctx, tx, e.Rep1)
			return err
		}},
		{"dependency snapshot", func(ctx context.Context, tx pgx.Tx) error {
			_, err := assignments.InspectMemberAssignmentsTx(ctx, tx, e.Rep1, nil)
			return err
		}},
		{"end archived responsibilities", func(ctx context.Context, tx pgx.Tx) error {
			return assignments.ArchiveRetiredMemberAssignmentsTx(ctx, tx, e.Rep1)
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := integration.OnSeat(e.As(e.AdminUser, nil, principal.Permissions{}), principal.SeatFull)
			err := e.DB().Tx(ctx, func(tx pgx.Tx) error { return scenario.run(ctx, tx) })
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("metadata without member-administration grant: %v", err)
			}
		})
	}
}
