// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactMergePreviewShowsPrimaryAddressDemotions(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprintf("occupied=%t", occupied), func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
			source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
				FullName: "Source contact", Source: "manual",
				Emails: []contacts.ContactEmailInput{{Email: "source-work@example.test", EmailType: "work", IsPrimary: true}, {Email: "source-personal@example.test", EmailType: "personal", IsPrimary: true}},
				Phones: []contacts.ContactPhoneInput{{Phone: "+49301234001", PhoneType: "work", IsPrimary: true}, {Phone: "+49301234002", PhoneType: "home", IsPrimary: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			targetInput := contacts.CreateContactInput{FullName: "Surviving contact", Source: "manual"}
			if occupied {
				targetInput.Emails = []contacts.ContactEmailInput{{Email: "survivor@example.test", EmailType: "work", IsPrimary: true}}
				targetInput.Phones = []contacts.ContactPhoneInput{{Phone: "+49301234003", PhoneType: "work", IsPrimary: true}}
			}
			target, err := e.Contacts.CreateContact(ctx, targetInput)
			if err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := ids.UUID(source.Id), ids.UUID(target.Id)
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", sourceID, targetID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Addresses) != 4 {
				t.Fatalf("addresses=%+v", preview.Addresses)
			}
			for _, address := range preview.Addresses {
				primary := !occupied || address.Usage != "work"
				if !address.WasPrimary || address.WillBePrimary != primary {
					t.Errorf("address=%+v want primary=%t", address, primary)
				}
			}
			checkMergeAddressMasks(t, e, "contact", sourceID, targetID, []string{"email", "emails", "phone", "phones"})
			if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](sourceID), ids.From[ids.ContactKind](targetID), &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			checkMergedAddresses(ctx, t, e, targetID, preview.Addresses)
		})
	}
}

func TestCompanyMergePreviewShowsDomainDemotions(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprintf("occupied=%t", occupied), func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
			source, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Source company", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "source.test", IsPrimary: true}, {Domain: "secondary.test"}}})
			if err != nil {
				t.Fatal(err)
			}
			targetInput := contacts.CreateCompanyInput{DisplayName: "Surviving company", Source: "manual"}
			if occupied {
				targetInput.Domains = []contacts.CompanyDomainInput{{Domain: "survivor.test", IsPrimary: true}}
			}
			target, err := e.Contacts.CreateCompany(ctx, targetInput)
			if err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := ids.UUID(source.Id), ids.UUID(target.Id)
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "company", sourceID, targetID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Addresses) != 2 {
				t.Fatalf("addresses=%+v", preview.Addresses)
			}
			for _, address := range preview.Addresses {
				before := address.Value == "source.test"
				if address.Kind != "domain" || address.WasPrimary != before || address.WillBePrimary != (before && !occupied) {
					t.Errorf("address=%+v", address)
				}
			}
			checkMergeAddressMasks(t, e, "company", sourceID, targetID, []string{"domains"})
			if _, err := e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](sourceID), ids.From[ids.CompanyKind](targetID), &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			checkMergedAddresses(ctx, t, e, targetID, preview.Addresses)
		})
	}
}

func checkMergeAddressMasks(t *testing.T, e *integration.Env, kind string, source, target ids.UUID, fields []string) {
	t.Helper()
	for _, field := range fields {
		perms := integration.AdminPerms
		perms.RoleKeys = []string{"rep"}
		perms.RowScope = principal.RowScopeOwn
		perms.FieldMasks = []principal.FieldMask{{Object: kind, Field: field, Condition: principal.MaskAlways}}
		preview, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, perms), kind, source, target, nil, nil)
		if err == nil || preview.Fields != nil || preview.Addresses != nil || preview.SourceVersion != 0 {
			t.Fatalf("%s mask leaked %+v err=%v", field, preview, err)
		}
	}
}

func checkMergedAddresses(ctx context.Context, t *testing.T, e *integration.Env, target ids.UUID, addresses []contacts.MergeAddress) {
	t.Helper()
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for _, address := range addresses {
			var query string
			switch address.Kind {
			case "email":
				query = `SELECT email, is_primary FROM contact_email WHERE id=@id AND contact_id=@target AND archived_at IS NULL`
			case "phone":
				query = `SELECT phone, is_primary FROM contact_phone WHERE id=@id AND contact_id=@target AND archived_at IS NULL`
			case "domain":
				query = `SELECT domain, is_primary FROM company_domain WHERE id=@id AND company_id=@target AND archived_at IS NULL`
			default:
				return fmt.Errorf("unexpected address kind %s", address.Kind)
			}
			var value string
			var primary bool
			if err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{"id": address.ID, "target": target}).Scan(&value, &primary); err != nil {
				return err
			}
			if value != address.Value || primary != address.WillBePrimary {
				t.Errorf("%s value=%q primary=%t preview=%+v", address.Kind, value, primary, address)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
