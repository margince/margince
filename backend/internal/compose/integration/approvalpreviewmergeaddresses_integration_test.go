// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

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
	t.Parallel()
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprintf("occupied=%t", occupied), func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
			source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
				FullName: "Source contact", Source: "manual",
				Emails: []contacts.ContactEmailInput{{Email: "source-work@example.test", EmailType: "work", IsPrimary: true}, {Email: "source-personal@example.test", EmailType: "personal"}},
				Phones: []contacts.ContactPhoneInput{{Phone: "+49301234001", PhoneType: "work", IsPrimary: true}, {Phone: "+49301234002", PhoneType: "home"}},
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
				wasPrimary := address.Usage == "work"
				willBePrimary := wasPrimary && !occupied
				if address.WasPrimary != wasPrimary || address.WillBePrimary != willBePrimary {
					t.Errorf("address=%+v want was=%t will=%t", address, wasPrimary, willBePrimary)
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

// A number both contacts hold shows as held in the preview, and the merge then
// keeps the survivor's row of it alone. That row stays primary, or takes the
// source's primary flag when the survivor had no primary number.
func TestContactMergePreviewShowsANumberTheSurvivorHolds(t *testing.T) {
	t.Parallel()
	for _, survivorPrimary := range []bool{true, false} {
		t.Run(fmt.Sprintf("survivorPrimary=%t", survivorPrimary), func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
			source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
				FullName: "Held source", Source: "manual",
				Phones: []contacts.ContactPhoneInput{{Phone: "+49301235001", PhoneType: "mobile", IsPrimary: true}, {Phone: "+49301235002", PhoneType: "work", Position: 1}},
			})
			if err != nil {
				t.Fatal(err)
			}
			held := []contacts.ContactPhoneInput{{Phone: "+49301235001", PhoneType: "mobile", IsPrimary: true}}
			target, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Held survivor", Source: "manual", Phones: held})
			if err != nil {
				t.Fatal(err)
			}
			targetID := ids.From[ids.ContactKind](ids.UUID(target.Id))
			if !survivorPrimary {
				held[0].IsPrimary = false
				if _, err := e.Contacts.UpdateContact(ctx, targetID, contacts.UpdateContactInput{Phones: held}); err != nil {
					t.Fatal(err)
				}
			}
			sourceID := ids.UUID(source.Id)
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", sourceID, targetID.UUID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, address := range preview.Addresses {
				isHeld := address.Value == "+49301235001"
				if address.AlreadyHeld != isHeld || (isHeld && !address.WillBePrimary) {
					t.Errorf("address=%+v want held=%t, and the held number primary on the survivor", address, isHeld)
				}
			}
			if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](sourceID), targetID, &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			checkMergedAddresses(ctx, t, e, targetID.UUID, preview.Addresses)
		})
	}
}

func TestCompanyMergePreviewShowsDomainDemotions(t *testing.T) {
	t.Parallel()
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
				query = `SELECT email, is_primary, archived_at IS NOT NULL FROM contact_email WHERE id=@id AND contact_id=@target`
			case "phone":
				query = `SELECT phone, is_primary, archived_at IS NOT NULL FROM contact_phone WHERE id=@id AND contact_id=@target`
			case "domain":
				query = `SELECT domain, is_primary, archived_at IS NOT NULL FROM company_domain WHERE id=@id AND company_id=@target`
			default:
				return fmt.Errorf("unexpected address kind %s", address.Kind)
			}
			var value string
			var primary, archived bool
			if err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{"id": address.ID, "target": target}).Scan(&value, &primary, &archived); err != nil {
				return err
			}
			if archived {
				// A held number's primary flag lives on the survivor's own row of it.
				if err := tx.QueryRow(ctx, `SELECT bool_or(is_primary) FROM contact_phone WHERE contact_id=@target AND phone=@phone AND archived_at IS NULL`,
					pgx.StrictNamedArgs{"target": target, "phone": value}).Scan(&primary); err != nil {
					return err
				}
			}
			if value != address.Value || primary != address.WillBePrimary || archived != address.AlreadyHeld {
				t.Errorf("%s value=%q primary=%t preview=%+v", address.Kind, value, primary, address)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
