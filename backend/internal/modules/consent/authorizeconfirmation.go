// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Whether the record bears out a message the INSTALLATION sends about itself.
//
// Its own file beside authorizevalidators.go because the evidence is of a
// different kind. Every validator there reads a business record a rep created —
// an invoice, a contract, a thread. This one reads a live confirm_token, which
// only this module mints and only the installation can cause to exist.
//
// That difference is the whole safety of the lane. The send doors refuse any
// caller-claimed category where ServesTheSubject is true
// (activities/sendcontext.go), so a rep cannot dress marketing as a
// confirmation. What reaches here is the controller lane, and it still has to
// show the token: a claim on its own proves nothing, and a confirmation message
// carrying no live link is not a confirmation, it is unsolicited mail with a
// reassuring name.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// validateConfirmation answers a message that asks the subject to confirm
// something — their details, or an opt-in they chose.
//
// The evidence is a confirm_token for THIS contact, of the kind this category
// carries, still live. Live means unconsumed and unexpired, because a link that
// can no longer be followed makes the mail a dead end for the contact who gets
// it.
//
// The basis is a legal obligation rather than consent, and that ordering
// matters: asking somebody to check what is held about them is Art. 14 work the
// installation owes them, so it cannot rest on a permission they have not given
// yet. A consent confirmation is the same shape — the mail that ASKS for
// consent cannot itself require consent, or no one could ever be asked.
func validateConfirmation(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if subject.Kind != entityContact {
		// Only a contact holds a confirm_token: the table's foreign key says so.
		// A lead has no link to show and therefore no confirmation to send.
		return unsupported, nil
	}
	kind, ok := confirmKindFor(category)
	if !ok {
		return unsupported, nil
	}
	var live bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM confirm_token
		    WHERE contact_id = $1
		      AND kind = $2
		      AND consumed_at IS NULL
		      AND expires_at > now()
		)`, subject.ID, kind).Scan(&live); err != nil {
		return resolution{}, fmt.Errorf("consent: reading the confirmation link: %w", err)
	}
	if !live {
		return unsupported, nil
	}
	return resolution{
		Category:  category,
		Basis:     commsauthz.BasisLegalObligation,
		Supported: true,
	}, nil
}

// confirmKindFor maps a category to the confirm_token kind that evidences it.
//
// It is deliberately NOT total over the five subject-serving categories. A
// security notice carries no link and is not answered here; an opt-out
// acknowledgement is sent when a token has just been spent, so no live one
// remains to find. Those stay unsupported until each has evidence of its own,
// which is the fail-closed default this package keeps.
//
// A PRIVACY NOTICE used to be in that list, on the ground that it carries no
// link. It does now: the notice mail is a one-time link to a page showing what
// is held, where it came from and the rights over it, minted by
// IssuePrivacyNotice and stored in the same confirm_token table. The evidence
// is therefore the same evidence — a live unspent token of that kind — and
// leaving it out meant every notice the installation sent was refused for
// having none.
func confirmKindFor(category commsauthz.Category) (string, bool) {
	switch category {
	case commsauthz.CategoryRecordConfirmation:
		return LinkRecordConfirmation, true
	case commsauthz.CategoryConsentConfirmation:
		return LinkConsentConfirmation, true
	case commsauthz.CategoryPrivacyNotice:
		return LinkPrivacyNotice, true
	default:
		return "", false
	}
}

// validateOptOutAcknowledgement answers whether this contact is owed a
// confirmation that their refusal of advertising was received.
//
// THE EVIDENCE IS THE STOP ITSELF, which is what makes this validator a
// different shape from the confirmation one above. Those messages carry a link
// and the live link IS the evidence. An acknowledgement carries nothing to
// click, so what it must show is the thing it acknowledges: a standing
// suppression that refused advertising.
//
// WITHOUT THIS the message could never be sent. Its category falls through to
// the legacy verdict and is denied — the defect
// TestEveryControllerTemplateResolvesToASubjectServingCategory exists to catch,
// and which shipped once already on the privacy notice.
//
// A LIFTED STOP EVIDENCES NOTHING. Somebody whose objection was withdrawn is
// not owed an acknowledgement of it, and sending one would tell them their
// advertising is stopped when it is not.
func validateOptOutAcknowledgement(
	ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category,
) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if subject.Kind != entityContact {
		return unsupported, nil
	}
	var standing bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM communication_suppression
		    WHERE contact_id = $1
		      AND kind = ANY($2)
		      AND revoked_at IS NULL
		)`, subject.ID, acknowledgeableKinds()).Scan(&standing); err != nil {
		return resolution{}, fmt.Errorf("consent: reading the stop this would acknowledge: %w", err)
	}
	if !standing {
		return unsupported, nil
	}
	return resolution{
		Category:  category,
		Basis:     commsauthz.BasisLegalObligation,
		Supported: true,
	}, nil
}
