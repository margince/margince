// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a message IS, worked out from where it came from rather than from the
// label its sender put on it.
//
// The old model asked one question — which consent_purpose was named — and a
// purpose is a caller-supplied string. `transactional` was an unconditional
// allow, so any message that called itself operational was one. That is the
// escape hatch this file closes: a caller may CLAIM a category, and the engine
// decides which one the record actually supports.
//
// Resolution is separate from authorization on purpose. This file answers "what
// kind of message is this", and the validators answer "is there evidence for
// that kind". Keeping them apart is what lets a decision row record a claim the
// engine disagreed with, which is the disagreement observe mode exists to make
// visible.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// resolution is what the engine concluded a message is, and why.
type resolution struct {
	// Category is what the message actually is, which may not be what the
	// caller claimed.
	Category commsauthz.Category
	// Basis is the lawful ground the category rests on when it is supported.
	Basis commsauthz.Basis
	// Supported reports whether the record bears the category out. False means
	// the decision is a review with Reason naming what is missing — never a
	// silent downgrade to something weaker that happens to be allowed.
	Supported bool
	// Reason is the code an unsupported resolution carries.
	Reason string
}

// resolveCategory works out what this message is for one recipient.
//
// The ORDER is the rule, and it runs from strongest evidence to weakest:
//
//  1. A thread the subject started. If this recipient is on the anchor's
//     thread, the message is a reply whatever anyone claimed — the subject
//     wrote to us and has not withdrawn, and that is the strongest ground a
//     message can have. It is checked first so a rep who mislabels a reply
//     still gets a reply.
//  2. A live deal in the links. An opportunity the recipient is a stakeholder
//     on supports an unprompted follow-up.
//  3. The claimed category, where the claim is one the caller may make and the
//     evidence bears out.
//  4. The legacy purpose class, mapped conservatively.
//
// Nothing here consults the caller's word before the record. A claim is
// evidence about intent and never about lawfulness, so it is checked against
// the tables rather than believed.
func (g *Gate) resolveCategory(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	if req.AnchorActivityID != (ids.UUID{}) {
		replied, err := repliesToTheSubject(ctx, tx, req.AnchorActivityID, subject)
		if err != nil {
			return resolution{}, err
		}
		if replied {
			return resolution{
				Category:  commsauthz.CategoryReplyToInbound,
				Basis:     commsauthz.BasisSubjectInitiatedCorrespondence,
				Supported: true,
			}, nil
		}
	}
	live, err := liveDealInLinks(ctx, tx, req.Links, subject)
	if err != nil {
		return resolution{}, err
	}
	if live {
		return resolution{
			Category:  commsauthz.CategoryActiveDealFollowup,
			Basis:     commsauthz.BasisLegitimateInterests,
			Supported: true,
		}, nil
	}
	return resolveFromClaimAndPurpose(ctx, tx, req)
}

// resolveFromClaimAndPurpose is the fallback when no thread and no live deal
// support the message: the caller's claim, then the legacy purpose.
//
// A claim the record does not bear out becomes a REVIEW naming what is missing,
// never a quiet downgrade to a weaker category that happens to be allowed. A
// rep who says "this is an invoice" and has no invoice should be told so; being
// silently re-labelled as marketing and refused for want of consent would send
// them looking in entirely the wrong place.
func resolveFromClaimAndPurpose(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (resolution, error) {
	if req.Context != "" {
		return resolution{
			Category: req.Context,
			// Unsupported until a validator says otherwise. The validators land
			// beside this and each one names the evidence its category needs;
			// until then a claim with no thread and no deal behind it is
			// exactly what a human should look at.
			Supported: false,
			Reason:    commsauthz.ReasonNoEvidence,
		}, nil
	}
	purpose, defined, err := purposeRowFor(ctx, tx, req.LegacyPurposeKey)
	if err != nil {
		return resolution{}, err
	}
	if !defined {
		return resolution{
			Category:  commsauthz.CategoryMarketing,
			Supported: false,
			Reason:    commsauthz.ReasonUnknownPurpose,
		}, nil
	}
	return resolutionForClass(purpose.Class), nil
}

// resolutionForClass maps the legacy purpose class conservatively.
//
// The transactional class is the one that changes meaning here. It used to be
// an unconditional allow — the escape hatch — and it now resolves to an
// account notice that is NOT supported: an operational claim with no invoice,
// contract or account event behind it is precisely the shape the old model
// could not tell from a cold sales mail. Marking it unsupported records the
// disagreement without refusing anything while the engine is observed.
func resolutionForClass(class Class) resolution {
	switch class {
	case ClassBusinessCorrespondence:
		// The class says correspondence and no thread was found, so what
		// supports it is a qualifying event the verdict path reads. Left
		// unsupported here so the evidence answer comes from one place.
		return resolution{
			Category:  commsauthz.CategoryReplyToInbound,
			Basis:     commsauthz.BasisSubjectInitiatedCorrespondence,
			Supported: false,
			Reason:    commsauthz.ReasonNoEvidence,
		}
	case ClassTransactional:
		return resolution{
			Category:  commsauthz.CategoryAccountNotice,
			Basis:     commsauthz.BasisContract,
			Supported: false,
			Reason:    commsauthz.ReasonLegacyTransactionalUnevidenced,
		}
	default:
		return resolution{
			Category:  commsauthz.CategoryMarketing,
			Basis:     commsauthz.BasisConsent,
			Supported: false,
			Reason:    commsauthz.ReasonNoMarketingConsent,
		}
	}
}

// stagedRequest rebuilds enough of the original question for the transmit phase
// to re-ask it.
//
// The transmit request carries no anchor, no links and no claimed context — the
// dispatcher holds a delivery row, not the compose window that produced it. So
// the staging decision is read back: it recorded what the caller claimed and
// what the engine resolved, keyed by this delivery.
//
// Re-asking rather than trusting the stored answer is the point. A staging
// decision says the message WAS a reply when it was written; between then and
// now the thread can be archived, the deal can close, the stakeholder can be
// removed. The category is carried forward as the claim, and the evidence is
// looked at again — so a message that stopped being what it was gets a fresh
// review rather than riding an answer that has expired.
func stagedRequest(ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest) (commsauthz.Request, error) {
	out := commsauthz.Request{
		Recipients:       req.Recipients,
		LegacyPurposeKey: req.PurposeKey,
		Subject:          req.Subject,
		Body:             req.Body,
	}
	var claimed, resolved *string
	err := tx.QueryRow(ctx, `
		SELECT requested_category, resolved_category
		  FROM communication_decision
		 WHERE delivery_id = $1 AND phase = 'staging'
		 ORDER BY decided_at DESC
		 LIMIT 1`, req.DeliveryID).Scan(&claimed, &resolved)
	if errors.Is(err, pgx.ErrNoRows) {
		// No staging row: a delivery staged before the engine existed, or one
		// whose staging decision was never written. The purpose key is all
		// there is, which is exactly what the engine had before resolution —
		// so it answers on that rather than refusing a message for the sake of
		// a row that predates the question.
		return out, nil
	}
	if err != nil {
		return commsauthz.Request{}, fmt.Errorf("consent: read what this delivery was staged as: %w", err)
	}
	// The CLAIM is carried forward, and the resolution is not. Carrying the
	// resolution would let the transmit phase inherit an allow the record no
	// longer supports; carrying the claim asks the same question again.
	if claimed != nil {
		out.Context = commsauthz.Category(*claimed)
	} else if resolved != nil {
		// Nothing was claimed, so the engine's own earlier reading stands in as
		// the claim. It is still only a claim here: the evidence decides.
		out.Context = commsauthz.Category(*resolved)
	}
	return out, nil
}
