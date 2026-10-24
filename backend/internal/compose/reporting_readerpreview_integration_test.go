// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withRoleAdmin gives the reporting fixture's seat the role matrix's read.
func withRoleAdmin(ctx context.Context) context.Context {
	actor, _ := principal.Actor(ctx)
	objects := maps.Clone(actor.Permissions.Objects)
	objects["role_admin"] = principal.ObjectGrant{Read: true}
	actor.Permissions.Objects = objects
	return principal.WithActor(ctx, actor)
}

// A preview under a role is the caller's access narrowed to the role's. A rep
// reads saved reports but measures only their own records, so a company-wide
// report is refused as it would be to a rep. Under the admin role the preview
// answers what the caller reads. A caller who may not read the role matrix
// gets no preview at all.
func TestAReportPreviewedAsARoleReadsWhatThatRoleMay(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
		Name: "Company review", Audience: "workspace", Selection: f.selection(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(f.writer, f.env.Pool, func(tx pgx.Tx) error {
		_, err := identity.SeedSystemRoles(f.writer, tx)
		return err
	}); err != nil {
		t.Fatalf("seeding the shipped roles: %v", err)
	}
	roles := identity.NewServiceFor(InstallationDB(f.env.Pool))
	preview := func(ctx context.Context, role string) (crmcontracts.ReportingEvaluation, error) {
		narrowed, err := asRole(ctx, roles, role)
		if err != nil {
			return crmcontracts.ReportingEvaluation{}, err
		}
		return f.service.EvaluateReport(narrowed, ids.UUID(report.Id), nil)
	}

	if _, err := preview(f.human, "admin"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without the role matrix previewed: %v, want permission denied", err)
	}
	admin := withRoleAdmin(f.human)
	own, err := f.service.EvaluateReport(admin, ids.UUID(report.Id), nil)
	if err != nil {
		t.Fatal(err)
	}
	asAdmin, err := preview(admin, "admin")
	if err != nil {
		t.Fatalf("preview as admin: %v", err)
	}
	if len(asAdmin.Metrics) != len(own.Metrics) || asAdmin.Metrics[0].Value == nil ||
		own.Metrics[0].Value == nil || *asAdmin.Metrics[0].Value != *own.Metrics[0].Value {
		t.Errorf("as admin the report reads %+v, want the caller's own figures %+v", asAdmin.Metrics, own.Metrics)
	}
	asRep, err := preview(admin, "rep")
	if err != nil {
		t.Fatalf("a rep may read the company totals, but the preview was refused: %v", err)
	}
	if len(asRep.Context.MemberIds) > 1 {
		t.Errorf("a rep's preview lists %d people, want only the ones a rep may measure", len(asRep.Context.MemberIds))
	}
	if _, err := preview(admin, "no-such-role"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an unknown role answered %v, want not found", err)
	}
}
