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
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
//
// WHERE THE ANCHOR AND LINKS COME FROM matters, because both admit a message on
// their own. They are not raw request fields by the time they reach here: the
// send path resolves them through SendOrigin.resolve, which reads the anchor
// with GetActivity under the caller's row scope and puts every named record
// through auth.EnsureLinkTarget — deliberately BEFORE the consent gate, so a
// caller naming a record they cannot see is refused there rather than reaching
// a gate that answers about recipients. A future caller that reaches this
// without that probe would be handing the engine unvalidated ids, and both
// supported arms would then be reachable by naming a stranger's deal.
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

// stagedClaims recovers what each recipient's message was CLAIMED to be, so
// the transmit phase can re-ask the same question.
//
// The transmit request carries no anchor, no links and no claimed context — the
// dispatcher holds a delivery row, not the compose window that produced it. So
// the staging decisions are read back, and they are read back PER RECIPIENT:
// AuthorizeStagingTx writes one row each, and one message can be several things
// at once. A single row taken for the whole delivery would judge every
// recipient by whichever one the query happened to return.
//
// Only the CLAIM is carried forward, never the engine's earlier resolution.
// Carrying the resolution would let a message ride an answer the record no
// longer supports — a thread can be archived and a deal can close while a
// delivery waits in the queue — and it would carry one recipient's answer onto
// another's.
func stagedClaims(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID) (map[string]commsauthz.Category, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (recipient_address) recipient_address, requested_category
		  FROM communication_decision
		 WHERE delivery_id = $1 AND phase = 'staging'
		 -- id, not decided_at: every row of one staging transaction carries the
		 -- SAME now(), so ordering by time is a tie the planner breaks however
		 -- it likes. uuidv7 is monotonic, so this names the newest row.
		 ORDER BY recipient_address, id DESC`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("consent: read what this delivery was staged as: %w", err)
	}
	defer rows.Close()

	claims := map[string]commsauthz.Category{}
	for rows.Next() {
		var address string
		var claimed *string
		if err := rows.Scan(&address, &claimed); err != nil {
			return nil, fmt.Errorf("consent: read what this delivery was staged as: %w", err)
		}
		if claimed != nil {
			claims[address] = commsauthz.Category(*claimed)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consent: read what this delivery was staged as: %w", err)
	}
	return claims, nil
}

// stagedRequestFor is the question to ask about ONE recipient at transmit.
//
// A recipient with no staged claim gets none: the purpose key is all there is,
// which is exactly what the engine had before resolution existed. That is the
// right answer for a delivery staged before this code shipped, and it refuses
// nothing that used to send.
func stagedRequestFor(req commsauthz.TransmitRequest, r connector.Recipient, claims map[string]commsauthz.Category) commsauthz.Request {
	return commsauthz.Request{
		Recipients:       []connector.Recipient{r},
		Context:          claims[decisionRecipientKey(r)],
		LegacyPurposeKey: req.PurposeKey,
		Subject:          req.Subject,
		Body:             req.Body,
	}
}
