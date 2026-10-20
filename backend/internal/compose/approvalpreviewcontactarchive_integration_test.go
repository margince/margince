// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestContactArchivePreviewNamesChannelsAndRelationshipsWithoutErasingHistory(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			ctx := e.As(e.AdminUser, nil, perms)
			contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Archive buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "buyer@example.test", EmailType: "work"}}, Phones: []contacts.ContactPhoneInput{{Phone: "+493012345678", PhoneType: "work"}}})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				bound, err := contacts.ResolveOrCreateChannelIdentity(ctx, tx, id, connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "opaque-test-account", Username: "synthetic-buyer"})
				if err == nil && bound != id {
					t.Error("identity bound to another contact")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			company := e.SeedCompany(t, "Retained company", &e.AdminUser)
			edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &id, CompanyID: new(ids.From[ids.CompanyKind](company)), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Retained conversation"), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: id.UUID}}})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"record_type":"contact","id":%q}`, id)
			if rest {
				payload = fmt.Sprintf(`{"operation":"archiveContact","path":"/v1/contacts/%s","body":null}`, id)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approval, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: "contact", TargetID: id.UUID, ProposedChange: raw, DiffHash: hash})
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
				if detail.Value.Text == "opaque-test-account" || detail.Value.Text == "synthetic-buyer" {
					t.Fatal("raw account identity disclosed")
				}
			}
			for field, want := range map[string]string{"archive_email": "buyer@example.test", "archive_phone": "+493012345678", "archive_channel": "telegram", "relationship_kind": "employment", "company_id": "Retained company"} {
				if fields[field] != want {
					t.Fatalf("%s=%q want %q", field, fields[field], want)
				}
			}
			for _, field := range []string{"emails", "phones", "reachability"} {
				masked := perms
				masked.RoleKeys = []string{"rep"}
				masked.RowScope = principal.RowScopeOwn
				masked.FieldMasks = []principal.FieldMask{{Object: "contact", Field: field, Condition: principal.MaskAlways}}
				hidden, err := svc.GetWire(e.As(e.AdminUser, nil, masked), approval)
				if err != nil {
					t.Fatal(err)
				}
				if hidden.Preview == nil || hidden.Preview.State == "ready" || hidden.Preview.Details != nil {
					t.Fatalf("%s mask leaked scope: %+v", field, hidden.Preview)
				}
			}
			if _, err := e.Contacts.ArchiveContact(ctx, id, new(int64(*wire.TargetVersion))); err != nil {
				t.Fatal(err)
			}
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				var liveChildren int
				if err := tx.QueryRow(ctx, `SELECT
     (SELECT count(*) FROM contact_email WHERE contact_id=@contact AND archived_at IS NULL)+
     (SELECT count(*) FROM contact_phone WHERE contact_id=@contact AND archived_at IS NULL)+
     (SELECT count(*) FROM contact_channel_identity WHERE contact_id=@contact AND archived_at IS NULL)+
     (SELECT count(*) FROM relationship WHERE id=@edge AND archived_at IS NULL)`, pgx.StrictNamedArgs{"contact": id, "edge": edge.ID}).Scan(&liveChildren); err != nil {
					return err
				}
				if liveChildren != 0 {
					t.Errorf("%d described children remain live", liveChildren)
				}
				var keptCompany, keptActivity bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM company WHERE id=@company AND archived_at IS NULL), EXISTS(SELECT 1 FROM activity WHERE id=@activity AND archived_at IS NULL)`, pgx.StrictNamedArgs{"company": company, "activity": activity.Id}).Scan(&keptCompany, &keptActivity); err != nil {
					return err
				}
				if !keptCompany || !keptActivity {
					t.Error("archive removed unrelated record or conversation")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
