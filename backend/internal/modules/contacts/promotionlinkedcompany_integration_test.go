// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func linkedPromotion(t *testing.T, existing bool) (*privacyEnv, context.Context, ids.LeadID, ids.CompanyID) {
	t.Helper()
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeTeam)
	company := e.linkCompany(ctx, t, "Demo Company")
	const email = "demo.buyer@gmail.com"
	var lead ids.LeadID
	if existing {
		if _, err := e.store.CreateContact(ctx, CreateContactInput{
			FullName: "Lena Lead", Source: "manual",
			Emails: []ContactEmailInput{{Email: email, EmailType: "personal", IsPrimary: true}},
		}); err != nil {
			t.Fatal(err)
		}
		lead = seedHistoricalLeadWithEmail(ctx, t, e.store, CreateLeadInput{
			FullName: new("Lena Lead"), CompanyName: new("Demo Company"), Source: "manual",
			OwnerID: new(ids.From[ids.UserKind](e.owner)),
		}, email)
	} else {
		lead = e.linkLead(ctx, t, e.owner, email, "Demo Company")
	}
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatal(err)
	}
	return e, ctx, lead, company
}

func assertLinkedLeadUnchanged(ctx context.Context, t *testing.T, e *privacyEnv, lead ids.LeadID, company ids.CompanyID, existing bool) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var linked *ids.UUID
		var retired bool
		if err := tx.QueryRow(ctx, `SELECT company_id,
			archived_at IS NOT NULL OR promoted_contact_id IS NOT NULL OR status = 'promoted'
			FROM lead WHERE id = @id`, pgx.StrictNamedArgs{"id": lead}).Scan(&linked, &retired); err != nil {
			return err
		}
		if linked == nil || *linked != company.UUID || retired {
			t.Errorf("refused conversion changed the lead: company=%v, retired=%v", linked, retired)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := 0
	if existing {
		want = 1
	}
	if contacts := e.contactsHolding(ctx, t, "demo.buyer@gmail.com"); len(contacts) != want {
		t.Fatalf("refused conversion left %d contacts, want %d", len(contacts), want)
	}
}

func TestLinkedLeadConversionCarriesCompanyOrLeavesTheLeadIntact(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, condition := range []string{"allowed", "relationship permission", "contact update permission", "company read permission", "hidden company", "archived company"} {
			t.Run(fmt.Sprintf("existing=%t/%s", existing, condition), func(t *testing.T) {
				e, owner, lead, company := linkedPromotion(t, existing)
				grants := map[string]principal.ObjectGrant{
					"lead": {Read: true, Update: true}, "contact": {Create: true, Read: true, Update: true},
					"company": {Create: true, Read: true, Update: true}, "relationship": {Create: true, Read: true, Update: true},
				}
				converter := e.owner
				wantErr := apperrors.ErrPermissionDenied
				switch condition {
				case "relationship permission":
					grants["relationship"] = principal.ObjectGrant{Read: true}
				case "contact update permission":
					grants["contact"] = principal.ObjectGrant{Read: true, Create: true}
				case "company read permission":
					delete(grants, "company")
				case "hidden company":
					if _, err := e.store.UpdateCompany(owner, company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
						t.Fatal(err)
					}
					converter, wantErr = e.teammate, apperrors.ErrNotFound
				case "archived company":
					if _, err := e.store.ArchiveCompany(owner, company, nil); err != nil {
						t.Fatal(err)
					}
					wantErr = apperrors.ErrNotFound
				}
				corr := ids.NewV7()
				ctx := principal.WithCorrelationID(e.linkerWith(converter, principal.RowScopeTeam, grants), corr)
				out, err := e.store.QualifyLead(ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
				if condition == "allowed" {
					if err != nil || out.Merged != existing {
						t.Fatalf("conversion: merged=%v, error=%v", out.Merged, err)
					}
					if !e.employed(owner, t, ids.From[ids.ContactKind](ids.UUID(out.Contact.Id)), company) {
						t.Fatal("converted contact lost the linked company")
					}
					return
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("conversion: %v, want %v", err, wantErr)
				}
				assertLinkedLeadUnchanged(owner, t, e, lead, company, existing)
				if events := e.eventTypesOf(owner, t, corr); len(events) != 0 {
					t.Fatalf("refused conversion emitted events: %v", events)
				}
			})
		}
	}
}

func TestPromotionPreviewsAndDigestRefuseAnUnavailableLinkedCompany(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprintf("archived=%t", archived), func(t *testing.T) {
			e, owner, lead, company := linkedPromotion(t, false)
			reader := owner
			if archived {
				if _, err := e.store.ArchiveCompany(owner, company, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := e.store.UpdateCompany(owner, company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
					t.Fatal(err)
				}
				reader = e.linker(e.teammate, principal.RowScopeTeam)
			}
			if _, err := e.store.GetLead(reader, lead, storekit.LiveOnly); err != nil {
				t.Fatalf("lead itself must remain readable: %v", err)
			}
			input := PromoteLeadInput{Trigger: "human_qualify"}
			if _, err := e.store.PreviewLeadContactChange(reader, lead, input); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("approval preview: %v, want company refusal", err)
			}
			if _, err := e.store.PreviewLeadPromotionWithOptions(reader, lead, PromotionPreviewOptions{}); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("lead-page preview: %v, want company refusal", err)
			}
			err := e.store.tx(reader, func(tx pgx.Tx) error {
				_, err := e.store.PromotionContextDigestTx(reader, tx, lead, input)
				return err
			})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("approval digest: %v, want company refusal", err)
			}
		})
	}
}

func TestContactCreationCannotDropTheCompanyOfALeadItConverts(t *testing.T) {
	e, owner, lead, company := linkedPromotion(t, false)
	if _, err := e.store.UpdateCompany(owner, company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	reader := e.linker(e.teammate, principal.RowScopeTeam)
	_, err := e.store.CreateContact(reader, CreateContactInput{
		FullName: "Lena Lead", Source: "manual",
		Emails:         []ContactEmailInput{{Email: "demo.buyer@gmail.com", EmailType: "personal", IsPrimary: true}},
		ConvertLeadIDs: []ids.LeadID{lead},
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("create and convert: %v, want company refusal", err)
	}
	assertLinkedLeadUnchanged(owner, t, e, lead, company, false)
}

func TestPromotionContextReadDoesNotTakeACompanyLock(t *testing.T) {
	e, owner, lead, company := linkedPromotion(t, false)
	holder, _ := beginOn(t)
	if _, err := holder.Exec(owner, `SELECT id FROM company WHERE id = @id FOR UPDATE`, pgx.StrictNamedArgs{"id": company}); err != nil {
		t.Fatal(err)
	}
	err := e.store.tx(owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(owner, "SET LOCAL lock_timeout = '100ms'"); err != nil {
			return err
		}
		digest, err := e.store.PromotionContextDigestTx(owner, tx, lead, PromoteLeadInput{Trigger: "human_qualify"})
		if err == nil && digest == "" {
			t.Error("promotion context was not read")
		}
		return err
	})
	if err != nil {
		t.Fatalf("read-only promotion context waited on a company write: %v", err)
	}
}
