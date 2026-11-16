// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The warning before a send to a recipient whose pack has no legitimate-interest
// basis. It applies when the record holds neither their consent nor an
// agreement with them.
//
// It does not block. The preview marks the recipient, the screen shows the
// warning, and the person who sends confirms it here first. The confirmation
// is an audit row against the recipient's record that names that person.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// lawfulBasisWarning names the warning in the audit row's after image.
const lawfulBasisWarning = "lawful_basis_missing"

// maxConfirmedAddresses mirrors the contract's maxItems.
const maxConfirmedAddresses = 100

// lawfulUseMissingTx answers whether a decided recipient falls under a pack
// with no legitimate-interest basis and has no lawful use on record.
func lawfulUseMissingTx(ctx context.Context, tx pgx.Tx, r connector.Recipient, req commsauthz.Request, d commsauthz.Decision) (bool, error) {
	if d.SubjectKind == "" {
		return false, nil
	}
	country, err := recipientRequestCountry(ctx, tx, r, req)
	if err != nil {
		return false, err
	}
	return lawfulUseMissingFor(ctx, tx, subjectRef{Kind: d.SubjectKind, ID: d.SubjectID.String(), Country: country})
}

func lawfulUseMissingFor(ctx context.Context, tx pgx.Tx, subject subjectRef) (bool, error) {
	rules, _, found := applicableRules(subject.Country.Code)
	if !found || !rules.NoLegitimateInterests {
		return false, nil
	}
	basis, err := lawfulUseOnRecordTx(ctx, tx, subject)
	return basis == "", err
}

// ConfirmSendWithoutLawfulBasis records that the signed-in person read the
// warning for each address and chose to send. It answers the addresses the
// warning still applied to; the rest need no confirmation.
func (s *Store) ConfirmSendWithoutLawfulBasis(ctx context.Context, addresses []string) ([]string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, apperrors.ErrPermissionDenied
	}
	if len(addresses) == 0 || len(addresses) > maxConfirmedAddresses {
		return nil, &ValidationError{Field: "addresses", Reason: fmt.Sprintf("name 1 to %d addresses", maxConfirmedAddresses)}
	}
	confirmed := []string{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, address := range addresses {
			applies, err := confirmOneTx(ctx, tx, address)
			if err != nil {
				return err
			}
			if applies {
				confirmed = append(confirmed, address)
			}
		}
		return nil
	})
	return confirmed, err
}

func confirmOneTx(ctx context.Context, tx pgx.Tx, address string) (bool, error) {
	who, err := subjectOfRecipient(ctx, tx, connector.Recipient{Email: address})
	if err != nil {
		return false, err
	}
	if who.Kind == "" {
		return false, fmt.Errorf("consent: no record holds this address: %w", apperrors.ErrNotFound)
	}
	id, err := ids.Parse(who.ID)
	if err != nil {
		return false, fmt.Errorf("consent: the resolved subject is not an id: %w", err)
	}
	if err := auth.Require(ctx, who.Kind, principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.EnsureVisible(ctx, tx, who.Kind, id); err != nil {
		return false, err
	}
	country, err := recipientCountryTx(ctx, tx, who)
	if err != nil {
		return false, err
	}
	missing, err := lawfulUseMissingFor(ctx, tx, subjectRef{Kind: who.Kind, ID: who.ID, Country: country})
	if err != nil || !missing {
		return false, err
	}
	_, err = storekit.AuditEvent(ctx, tx, "approve", who.Kind, id, map[string]any{
		"warning": lawfulBasisWarning, "address": address, "country": string(country.Code),
	})
	return err == nil, err
}

// ConfirmSendWithoutLawfulBasis serves POST /emails/lawful-basis-confirmations.
func (h Handlers) ConfirmSendWithoutLawfulBasis(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.LawfulBasisConfirmationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	addresses := make([]string, 0, len(req.Addresses))
	for _, a := range req.Addresses {
		addresses = append(addresses, string(a))
	}
	confirmed, err := h.store.ConfirmSendWithoutLawfulBasis(r.Context(), addresses)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.LawfulBasisConfirmation{Confirmed: make([]openapi_types.Email, 0, len(confirmed))}
	for _, a := range confirmed {
		out.Confirmed = append(out.Confirmed, openapi_types.Email(a))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
