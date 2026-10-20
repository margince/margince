// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestLeadCarryPreviewNamesTheRecordedBasisWithoutChangingItsValidity(t *testing.T) {
	e := setupChannelConsent(t)
	lead := seedLead(t, e, "Dana Buyer", "basis@example.test")
	ctx := principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"lead": {Read: true, Update: true}, "contact": {Read: true, Create: true, Update: true},
		}}})
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return recordBasis(ctx, tx, subjectRef{Kind: entityLead, ID: lead.String()},
			resolution{Supported: true, Basis: commsauthz.BasisSubjectInitiatedCorrespondence}, commsauthz.Request{}, packRules{reply: 24 * time.Hour})
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		preview, err := e.store.PreviewConsentLinksTx(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(e.contact))
		if err != nil {
			return err
		}
		if len(preview.CommunicationBases) != 1 {
			t.Fatalf("basis scope: %+v", preview)
		}
		basis := preview.CommunicationBases[0]
		if basis.Kind != string(commsauthz.BasisSubjectInitiatedCorrespondence) || basis.ValidUntil == nil || !basis.ValidUntil.After(basis.ValidFrom) || basis.RevokedAt != nil {
			t.Fatalf("basis: %+v", basis)
		}
		if err := e.store.CarrySatellitesTx(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(e.contact)); err != nil {
			return err
		}
		var same bool
		if err := tx.QueryRow(ctx, `SELECT contact_id = @contact AND lead_id IS NULL AND kind = @kind AND valid_from = @from AND valid_until = @until AND revoked_at IS NULL FROM communication_basis WHERE id = @id`,
			pgx.NamedArgs{"contact": e.contact, "kind": basis.Kind, "from": basis.ValidFrom, "until": basis.ValidUntil, "id": basis.ID}).Scan(&same); err != nil {
			return err
		}
		if !same {
			t.Fatal("carried basis differs from preview")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCarryPreviewRefusesAnOversizedScopeWithoutReturningPartialLinks(t *testing.T) {
	e := setupChannelConsent(t)
	lead := seedLead(t, e, "Large history", "history@example.test")
	ctx := principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"lead": {Read: true, Update: true}, "contact": {Read: true, Create: true, Update: true},
		}}})
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		for i := range 501 {
			if _, err := e.store.EnsureWithdrawalCredentialTx(ctx, tx, WithdrawalMintInput{
				LeadID: lead, Address: fmt.Sprintf("history-%d@example.test", i), Scope: WithdrawalScopeAllMarketing,
			}); err != nil {
				return err
			}
			if i == 499 {
				full, err := e.store.PreviewConsentLinksTx(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(e.contact))
				if err != nil {
					return err
				}
				if len(full.WithdrawalCredentials) != 500 {
					t.Fatalf("complete boundary scope has %d links", len(full.WithdrawalCredentials))
				}
			}
		}
		refused, err := e.store.PreviewConsentLinksTx(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(e.contact))
		if !errors.Is(err, apperrors.ErrInvalidArgument) || len(refused.WithdrawalCredentials) != 0 {
			t.Fatalf("oversized scope returned %+v, %v", refused, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
