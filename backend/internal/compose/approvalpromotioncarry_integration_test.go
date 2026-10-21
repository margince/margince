// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestPromotionPreviewNamesTheRestrictionsAndLinksActuallyCarried(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, perms)
	consents := consent.NewStore(e.DB())
	store := e.Contacts.WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
	lead, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Dana Buyer"), Email: new("carry@example.test"), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.AdminUser))})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	var token string
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		token, err = consents.EnsureWithdrawalCredentialTx(ctx, tx, consent.WithdrawalMintInput{LeadID: leadID, Address: "carry@example.test", Scope: consent.WithdrawalScopeAllMarketing})
		if err != nil {
			return err
		}
		_, err = consents.StopForCredentialTx(ctx, tx, consent.WithdrawalRef{LeadID: leadID, Address: "carry@example.test", Scope: consent.WithdrawalScopeAllMarketing})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in := contacts.PromoteLeadInput{Trigger: "human_qualify"}
	preview, err := store.PreviewLeadContactChange(ctx, leadID, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Stops) != 1 || preview.Stops[0].Kind != "marketing_objection" || preview.Stops[0].PurposeID != nil || len(preview.ConsentLinks.WithdrawalCredentials) != 1 {
		t.Fatalf("carried scope: %+v", preview)
	}
	link := preview.ConsentLinks.WithdrawalCredentials[0]
	if link.Address != "carry@example.test" || link.Scope != consent.WithdrawalScopeAllMarketing || !link.ExpiresAt.After(link.IssuedAt) || link.RevokedAt != nil {
		t.Fatalf("withdrawal review scope: %+v", link)
	}
	masked := perms
	masked.RoleKeys = []string{"rep"}
	masked.RowScope = principal.RowScopeOwn
	masked.FieldMasks = []principal.FieldMask{{Object: "lead", Field: "email", Condition: principal.MaskAlways}}
	hidden := e.As(e.AdminUser, nil, masked)
	if err := e.DB().Tx(hidden, func(tx pgx.Tx) error {
		links, err := consents.PreviewConsentLinksTx(hidden, tx, commsauthz.LeadStopSubject(leadID), commsauthz.ContactStopSubject(ids.ContactID{}))
		if !errors.Is(err, apperrors.ErrPermissionDenied) || len(links.WithdrawalCredentials) > 0 {
			t.Fatalf("masked address leaked: %+v, %v", links, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	details, err := newApprovalPreviewReader(e.DB()).promotionConsentLinkDetails(ctx, preview.ConsentLinks)
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 5 || details[0].Value.Text != link.Address || details[0].Value.EntityId == nil || ids.UUID(*details[0].Value.EntityId) != link.ID {
		t.Fatalf("reviewable links: %+v", details)
	}
	raw, hash, err := diffhash.Canonical([]byte(`{"operation":"PromoteLead","body":{"trigger":"human_qualify"}}`))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	proposalID, err := svc.Stage(ctx, approvals.StageInput{Kind: "promote_lead", TargetType: "lead", TargetID: leadID.UUID, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Effects == nil || !slices.Contains(*proposal.Preview.Effects, "promote_move_consent_links") {
		t.Fatalf("carried approval scope: %+v", proposal.Preview)
	}
	unwired := contacts.NewStore(e.DB())
	var missing *contacts.StopCarrierNotWiredError
	if _, err := unwired.PreviewLeadContactChange(ctx, leadID, in); !errors.As(err, &missing) {
		t.Fatalf("unwired stop preview: %v", err)
	}
	stopOnly := contacts.NewStore(e.DB()).WithStopCarrier(consents)
	var missingLinks *contacts.SatelliteCarrierNotWiredError
	if _, err := stopOnly.PreviewLeadContactChange(ctx, leadID, in); !errors.As(err, &missingLinks) {
		t.Fatalf("unwired links preview: %v", err)
	}
	contact, _, err := store.PromoteLead(ctx, leadID, in)
	if err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE contact_id = @contact AND carried_from = @source AND kind = @kind AND decided_by_level = @authority`, pgx.NamedArgs{"contact": ids.UUID(contact.Id), "source": preview.Stops[0].ID, "kind": preview.Stops[0].Kind, "authority": preview.Stops[0].Authority}) != 1 {
		t.Fatal("actual restriction differs from preview")
	}
	if e.WsCount(t, `SELECT count(*) FROM withdrawal_credential WHERE id = @credential AND contact_id = @contact AND lead_id IS NULL`, pgx.NamedArgs{"credential": preview.ConsentLinks.WithdrawalCredentials[0].ID, "contact": ids.UUID(contact.Id)}) != 1 {
		t.Fatal("previewed withdrawal link did not move")
	}
	ref, err := consents.ResolveWithdrawalToken(ctx, token)
	if err != nil || ref.ContactID.UUID != ids.UUID(contact.Id) || !ref.LeadID.IsZero() {
		t.Fatalf("original link no longer reaches promoted contact: %+v, %v", ref, err)
	}
}

func TestMergePreviewCarriesRestrictionsAndKeepsWithdrawalLinksUsable(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	consents := consent.NewStore(e.DB())
	store := e.Contacts.WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
	source, err := store.CreateContact(ctx, contacts.CreateContactInput{FullName: "Source Buyer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateContact(ctx, contacts.CreateContactInput{FullName: "Surviving Buyer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, targetID := ids.From[ids.ContactKind](ids.UUID(source.Id)), ids.From[ids.ContactKind](ids.UUID(target.Id))
	var token string
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		token, err = consents.EnsureWithdrawalCredentialTx(ctx, tx, consent.WithdrawalMintInput{ContactID: sourceID, Address: "carry@example.test", Scope: consent.WithdrawalScopeAllMarketing})
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := consents.Suppress(ctx, consent.SuppressInput{ContactID: sourceID, Kind: "marketing_objection", Reason: "Subject asked to stop marketing"}); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewMergeSurvivorship(ctx, "contact", sourceID.UUID, targetID.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Stops) != 1 || preview.Stops[0].Kind != "marketing_objection" || len(preview.ConsentLinks.WithdrawalCredentials) != 1 {
		t.Fatalf("merge carry scope: %+v", preview)
	}
	var missing *contacts.StopCarrierNotWiredError
	denied, err := contacts.NewStore(e.DB()).PreviewMergeSurvivorship(ctx, "contact", sourceID.UUID, targetID.UUID, nil, nil)
	if !errors.As(err, &missing) || len(denied.Stops) != 0 {
		t.Fatalf("unwired carry = %+v, %v", denied, err)
	}
	if _, err := store.MergeContact(ctx, sourceID, targetID, nil); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE contact_id=@target AND carried_from=@source AND kind=@kind AND decided_by_level=@authority`,
		pgx.NamedArgs{"target": targetID, "source": preview.Stops[0].ID, "kind": preview.Stops[0].Kind, "authority": preview.Stops[0].Authority}) != 1 {
		t.Fatal("merged restriction differs from preview")
	}
	if e.WsCount(t, `SELECT count(*) FROM withdrawal_credential WHERE id=@credential AND contact_id=@target AND lead_id IS NULL`,
		pgx.NamedArgs{"credential": preview.ConsentLinks.WithdrawalCredentials[0].ID, "target": targetID}) != 1 {
		t.Fatal("previewed credential did not move")
	}
	ref, err := consents.ResolveWithdrawalToken(ctx, token)
	if err != nil || ref.ContactID != targetID || !ref.LeadID.IsZero() {
		t.Fatalf("merged withdrawal link = %+v, %v", ref, err)
	}
}
