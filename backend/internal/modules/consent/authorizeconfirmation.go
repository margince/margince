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
// The evidence is a confirm_token for THIS person, of the kind this category
// carries, still live. Live means unconsumed and unexpired, because a link that
// can no longer be followed makes the mail a dead end for the person who gets
// it.
//
// The basis is a legal obligation rather than consent, and that ordering
// matters: asking somebody to check what is held about them is Art. 14 work the
// installation owes them, so it cannot rest on a permission they have not given
// yet. A consent confirmation is the same shape — the mail that ASKS for
// consent cannot itself require consent, or no one could ever be asked.
func validateConfirmation(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if subject.Kind != entityPerson {
		// Only a person holds a confirm_token: the table's foreign key says so.
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
		    WHERE person_id = $1
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
// security notice and a privacy notice carry no link and are not answered here;
// an opt-out acknowledgement is sent when a token has just been spent, so no
// live one remains to find. Those stay unsupported until each has evidence of
// its own, which is the fail-closed default this package keeps.
func confirmKindFor(category commsauthz.Category) (string, bool) {
	switch category {
	case commsauthz.CategoryRecordConfirmation:
		return LinkRecordConfirmation, true
	case commsauthz.CategoryConsentConfirmation:
		return LinkConsentConfirmation, true
	default:
		return "", false
	}
}
