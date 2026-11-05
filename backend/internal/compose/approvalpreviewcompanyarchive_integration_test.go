// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyArchivePreviewNamesItsRetiredScope(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"relationship", "partner"} {
				perms.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Archive company", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "archive-company.test", IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
			if _, err := e.Contacts.UpsertPartner(ctx, contacts.UpsertPartnerInput{CompanyID: id, PartnerRole: "consulting"}); err != nil {
				t.Fatal(err)
			}
			contact := e.SeedContact(t, "Keep contact", &e.AdminUser)
			edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: new(ids.From[ids.ContactKind](contact)), CompanyID: &id, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"record_type":"company","id":%q}`, id)
			if rest {
				payload = fmt.Sprintf(`{"operation":"archiveCompany","path":"/v1/companies/%s","body":null}`, id)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approval, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: "company", TargetID: id.UUID, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			wire, err := svc.GetWire(ctx, approval)
			if err != nil {
				t.Fatal(err)
			}
			if wire.Preview == nil || wire.Preview.State != "ready" || wire.Preview.Details == nil || wire.TargetVersion == nil {
				t.Fatalf("preview=%+v", wire.Preview)
			}
			fields := map[string]string{}
			for _, detail := range *wire.Preview.Details {
				fields[detail.Field] = detail.Value.Text
			}
			for field, want := range map[string]string{"archive_domain": "archive-company.test", "archive_company_type": "partner", "archive_partner": "Archive company", "contact_id": "Keep contact"} {
				if fields[field] != want {
					t.Errorf("%s=%q want %q", field, fields[field], want)
				}
			}
			denied := perms
			denied.Objects = maps.Clone(perms.Objects)
			denied.Objects["partner"] = principal.ObjectGrant{}
			hidden, err := svc.GetWire(e.As(e.AdminUser, nil, denied), approval)
			if err != nil {
				t.Fatal(err)
			}
			if hidden.Preview == nil || hidden.Preview.State == "ready" || hidden.Preview.Details != nil {
				t.Fatalf("partner scope leaked: %+v", hidden.Preview)
			}
			for _, field := range []string{"domains", "relationship_types"} {
				masked := perms
				masked.RoleKeys = []string{"rep"}
				masked.RowScope = principal.RowScopeOwn
				masked.FieldMasks = []principal.FieldMask{{Object: "company", Field: field, Condition: principal.MaskAlways}}
				hidden, err := svc.GetWire(e.As(e.AdminUser, nil, masked), approval)
				if err != nil {
					t.Fatal(err)
				}
				if hidden.Preview == nil || hidden.Preview.State == "ready" || hidden.Preview.Details != nil {
					t.Fatalf("%s mask leaked: %+v", field, hidden.Preview)
				}
			}
			if _, err := e.Contacts.ArchiveCompany(ctx, id, new(int64(*wire.TargetVersion))); err != nil {
				t.Fatal(err)
			}
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				var liveChildren int
				var contactKept bool
				if err := tx.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM company_domain WHERE company_id=@company AND archived_at IS NULL)+
      (SELECT count(*) FROM company_relationship_type WHERE company_id=@company AND archived_at IS NULL)+
      (SELECT count(*) FROM partner WHERE company_id=@company AND archived_at IS NULL)+
      (SELECT count(*) FROM relationship WHERE id=@edge AND archived_at IS NULL),
      EXISTS(SELECT 1 FROM contact WHERE id=@contact AND archived_at IS NULL)`, pgx.StrictNamedArgs{"company": id, "edge": edge.ID, "contact": contact}).Scan(&liveChildren, &contactKept); err != nil {
					return err
				}
				if liveChildren != 0 || !contactKept {
					t.Errorf("live children=%d contact preserved=%t", liveChildren, contactKept)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompanyArchivePreviewHonorsVersionAndProjectGuard(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Project company", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if company.Version == nil {
		t.Fatal("missing version")
	}
	version := int64(*company.Version)
	scope, err := e.Contacts.PreviewCompanyArchive(ctx, id, new(version+1))
	if !errors.Is(err, apperrors.ErrVersionSkew) || len(scope.Relationships) != 0 {
		t.Fatalf("stale scope=%+v err=%v", scope, err)
	}
	if _, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Keep project", CompanyID: id, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	scope, err = e.Contacts.PreviewCompanyArchive(ctx, id, nil)
	var stranded *contacts.SoleProjectCompanyError
	if !errors.As(err, &stranded) || len(scope.Relationships) != 0 {
		t.Fatalf("project scope=%+v err=%v", scope, err)
	}
	if _, err := e.Contacts.ArchiveCompany(ctx, id, nil); !errors.As(err, &stranded) {
		t.Fatalf("archive disagrees with preview: %v", err)
	}
}
