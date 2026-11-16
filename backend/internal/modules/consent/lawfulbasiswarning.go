// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The warning before a send to a recipient whose pack has no legitimate-interest
// basis. It applies when the record holds no basis that pack accepts for the
// use (settlePackBasisTx).
//
// The preview marks the recipient and the screen warns. The user sending
// confirms for each recipient's record, and staging spends one confirmation
// per record on the delivery it lets through. A send with no confirmation is
// refused with ReasonLawfulBasisUnconfirmed, which a recorded human decision
// may still direct. An agent cannot confirm, so its send goes to that review.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
)

// lawfulBasisWarning names the warning in the audit row's after image.
const lawfulBasisWarning = "lawful_basis_missing"

// maxConfirmedSubjects mirrors the contract's maxItems.
const maxConfirmedSubjects = 100

const fieldSubjects = "subjects"

// confirmationLifetime is how long a confirmation waits for its send. It
// answers the message being written, not one sent later.
const confirmationLifetime = 15 * time.Minute

// ConfirmedSubject is one record a confirmation names.
type ConfirmedSubject struct {
	Kind string
	ID   ids.UUID
}

// ConfirmSendWithoutLawfulBasis records that the signed-in user read the
// warning for each record and chose to send. It needs the authority to send.
// A record the warning no longer applies to is left out of the answer. A
// record already holding an open confirmation by this user keeps that one.
func (s *Store) ConfirmSendWithoutLawfulBasis(ctx context.Context, subjects []ConfirmedSubject) ([]ConfirmedSubject, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, apperrors.ErrPermissionDenied
	}
	if len(subjects) == 0 || len(subjects) > maxConfirmedSubjects {
		return nil, &ValidationError{Field: fieldSubjects, Reason: fmt.Sprintf("name 1 to %d records", maxConfirmedSubjects)}
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return nil, err
	}
	confirmed := []ConfirmedSubject{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, subject := range subjects {
			applies, err := s.confirmOneTx(ctx, tx, actor.UserID, subject)
			if err != nil {
				return err
			}
			if applies {
				confirmed = append(confirmed, subject)
			}
		}
		return nil
	})
	return confirmed, err
}

// requireSubjectVisible admits a record the caller may read, by its literal
// object name.
func requireSubjectVisible(ctx context.Context, tx pgx.Tx, subject ConfirmedSubject) error {
	switch subject.Kind {
	case entityContact:
		if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
			return err
		}
	case entityLead:
		if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
			return err
		}
	default:
		return &ValidationError{Field: fieldSubjects, Reason: "name a contact or a lead"}
	}
	return auth.EnsureVisible(ctx, tx, subject.Kind, subject.ID)
}

func (s *Store) confirmOneTx(ctx context.Context, tx pgx.Tx, user ids.UUID, subject ConfirmedSubject) (bool, error) {
	if err := requireSubjectVisible(ctx, tx, subject); err != nil {
		return false, err
	}
	who := ruleSubject{Kind: subject.Kind, ID: subject.ID.String()}
	country, err := recipientCountryTx(ctx, tx, who)
	if err != nil {
		return false, err
	}
	if rules, _, found := applicableRules(country.Code); !found || !rules.NoLegitimateInterests {
		return false, nil
	}
	column := confirmationColumn(subject.Kind)
	var open bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM lawful_basis_confirmation
		                WHERE `+column+` = $1 AND confirmed_by = $2 AND spent_at IS NULL AND expires_at > now())`,
		subject.ID, user).Scan(&open); err != nil {
		return false, fmt.Errorf("consent: reading the open confirmation: %w", err)
	}
	if open {
		return true, nil
	}
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO lawful_basis_confirmation (`+column+`, confirmed_by, expires_at)
		VALUES ($1, $2, now() + $3::interval) RETURNING id`,
		subject.ID, user, confirmationLifetime.String()).Scan(&id); err != nil {
		return false, fmt.Errorf("consent: recording the confirmation: %w", err)
	}
	_, err = storekit.AuditEvent(ctx, tx, "approve", subject.Kind, subject.ID, map[string]any{
		"warning": lawfulBasisWarning, "confirmation_id": id, "country": string(country.Code),
	})
	return err == nil, err
}

// confirmationColumn is the subject column of lawful_basis_confirmation, a
// compile-time literal for each kind.
func confirmationColumn(kind string) string {
	if kind == entityLead {
		return subjectColumnLead
	}
	return subjectColumnContact
}

// requireConfirmationTx answers a staging decision whose recipient has no
// lawful basis on record. It spends the open confirmation for that record by
// the sending user, or by the user an agent acts for. With none, the decision
// becomes a refusal a recorded human decision may still direct.
func requireConfirmationTx(ctx context.Context, tx pgx.Tx, d commsauthz.Decision, deliveryID ids.UUID) (commsauthz.Decision, error) {
	if !d.LawfulBasisMissing || d.Verdict != commsauthz.VerdictAllow || d.SubjectKind == "" {
		return d, nil
	}
	user := confirmingUser(ctx)
	if !user.IsZero() {
		var spent ids.UUID
		err := tx.QueryRow(ctx, `
			UPDATE lawful_basis_confirmation SET spent_at = now(), delivery_id = $3
			 WHERE id = (SELECT id FROM lawful_basis_confirmation
			              WHERE `+confirmationColumn(d.SubjectKind)+` = $1 AND confirmed_by = $2
			                AND spent_at IS NULL AND expires_at > now()
			              ORDER BY confirmed_at DESC LIMIT 1 FOR UPDATE)
			RETURNING id`, d.SubjectID, user, deliveryID).Scan(&spent)
		if err == nil {
			return d, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return d, fmt.Errorf("consent: spending the confirmation: %w", err)
		}
	}
	d.Verdict, d.ReasonCode, d.Basis = commsauthz.VerdictDeny, commsauthz.ReasonLawfulBasisUnconfirmed, ""
	return d, nil
}

// confirmingUser is the user whose confirmation a send may spend: the human
// sending, or the user an agent sends for. Anyone else has none.
func confirmingUser(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	switch {
	case !ok:
		return ids.UUID{}
	case actor.Type == principal.PrincipalHuman:
		return actor.UserID
	case actor.Type == principal.PrincipalAgent:
		return actor.OnBehalfOf
	}
	return ids.UUID{}
}

// ConfirmSendWithoutLawfulBasis serves POST /lawful-basis-confirmations.
func (h Handlers) ConfirmSendWithoutLawfulBasis(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.LawfulBasisConfirmationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	subjects := make([]ConfirmedSubject, 0, len(req.Subjects))
	for _, s := range req.Subjects {
		subjects = append(subjects, ConfirmedSubject{Kind: string(s.EntityType), ID: ids.UUID(s.EntityId)})
	}
	confirmed, err := h.store.ConfirmSendWithoutLawfulBasis(r.Context(), subjects)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.LawfulBasisConfirmation{Confirmed: make([]crmcontracts.CommunicationEvidenceSubject, 0, len(confirmed))}
	for _, c := range confirmed {
		out.Confirmed = append(out.Confirmed, crmcontracts.CommunicationEvidenceSubject{
			EntityType: crmcontracts.CommunicationEvidenceSubjectEntityType(c.Kind), EntityId: openapi_types.UUID(c.ID),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
