// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactMergePreviewMatchesTheSurvivorFields(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Source Contact", Title: new("Director"), Address: &crmcontracts.Address{City: new("Berlin")}, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Surviving Contact", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, targetID := ids.UUID(source.Id), ids.UUID(target.Id)
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", sourceID, targetID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fields := normalizedMergeFields(t, preview.Fields)
	if fields["title"] != "Director" || fields["address_city"] != "Berlin" || len(fields) != 7 {
		t.Fatalf("fields=%v", fields)
	}
	checkMergePreviewRefusals(t, e, "contact", sourceID, targetID, preview, "title")
	merged, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](sourceID), ids.From[ids.ContactKind](targetID), &preview.SourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Title == nil || *merged.Title != "Director" || merged.Address == nil || merged.Address.City == nil || *merged.Address.City != "Berlin" || merged.FullName != target.FullName {
		t.Fatalf("survivor=%+v", merged)
	}
	checkMergeAuditFields(ctx, t, e, "contact", targetID, fields)
}

func TestCompanyMergePreviewKeepsExistingFieldsAndWholeAddresses(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	source, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Source account", LegalName: new("Source legal name"), Description: new("Discard this description"), Industry: new("manufacturing"), Address: &crmcontracts.Address{City: new("Berlin"), PostalCode: new("10115")}, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Surviving account", Description: new("Keep this description"), Address: &crmcontracts.Address{City: new("Hamburg")}, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, targetID := ids.UUID(source.Id), ids.UUID(target.Id)
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "company", sourceID, targetID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fields := normalizedMergeFields(t, preview.Fields)
	if fields["legal_name"] != "Source legal name" || fields["industry"] != "manufacturing" || len(fields) != 2 {
		t.Fatalf("fields=%v", fields)
	}
	checkMergePreviewRefusals(t, e, "company", sourceID, targetID, preview, "industry")
	merged, err := e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](sourceID), ids.From[ids.CompanyKind](targetID), &preview.SourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LegalName == nil || *merged.LegalName != "Source legal name" || merged.Description == nil || *merged.Description != "Keep this description" || merged.Address == nil || merged.Address.City == nil || *merged.Address.City != "Hamburg" || merged.Address.PostalCode != nil {
		t.Fatalf("survivor=%+v", merged)
	}
	checkMergeAuditFields(ctx, t, e, "company", targetID, fields)
}

func normalizedMergeFields(t *testing.T, fields map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkMergePreviewRefusals(t *testing.T, e *integration.Env, kind string, source, target ids.UUID, preview contacts.MergeSurvivorship, maskedField string) {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	for _, versions := range [][2]int64{{preview.SourceVersion + 1, preview.TargetVersion}, {preview.SourceVersion, preview.TargetVersion + 1}} {
		got, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, &versions[0], &versions[1])
		if !errors.Is(err, apperrors.ErrVersionSkew) || got.Fields != nil {
			t.Fatalf("stale preview=%+v err=%v", got, err)
		}
	}
	masked := integration.AdminPerms
	masked.RoleKeys = []string{"rep"}
	masked.RowScope = principal.RowScopeOwn
	masked.FieldMasks = []principal.FieldMask{{Object: kind, Field: maskedField, Condition: principal.MaskAlways}}
	got, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, masked), kind, source, target, nil, nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || got.Fields != nil {
		t.Fatalf("masked preview=%+v err=%v", got, err)
	}
}

func checkMergeAuditFields(ctx context.Context, t *testing.T, e *integration.Env, kind string, target ids.UUID, fields map[string]any) {
	t.Helper()
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT after FROM audit_log WHERE entity_type=@kind AND entity_id=@target AND action='merge'`, pgx.StrictNamedArgs{"kind": kind, "target": target}).Scan(&raw); err != nil {
			return err
		}
		var audit map[string]any
		if err := json.Unmarshal(raw, &audit); err != nil {
			return err
		}
		for field, want := range fields {
			if audit[field] != want {
				t.Errorf("%s audit=%v preview=%v", field, audit[field], want)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSurvivorshipRequiresAuthorityOverBothRecords(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			seed := e.SeedContact
			if kind == "company" {
				seed = e.SeedCompany
			}
			own := seed(t, "Owned record", &e.Rep1)
			other := seed(t, "Colleague record", &e.Rep2)
			perms := integration.AdminPerms
			perms.RoleKeys = []string{"rep"}
			perms.RowScope = principal.RowScopeOwn
			ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
			for _, pair := range [][2]ids.UUID{{own, other}, {other, own}} {
				preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, pair[0], pair[1], nil, nil)
				if err == nil || preview.Fields != nil || preview.SourceVersion != 0 || preview.TargetVersion != 0 {
					t.Fatalf("unowned half disclosed: %+v err=%v", preview, err)
				}
			}
		})
	}
}
