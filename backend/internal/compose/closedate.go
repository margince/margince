// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Close-date correction wiring (formulas §11, B-E09.20): the deals
// module owns the nightly sweep and the approvals module owns the 🟡
// inbox — this file is the cross-module edge between them, injected
// here like every other one. The sweep stages kind
// "close_date_correction" through the adapter below; a human approval
// releases the confirm effect, which redeems the staging and applies
// the (possibly edited) date through the deals store's own gated
// update path.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeDateStager adapts the approvals service onto the deals module's
// CorrectionStager seam.
type closeDateStager struct {
	svc *approvals.Service
}

// The two payload keys the staging identity is drawn from. Named because the
// identity and the payload must spell them the same way — canonicalIdentity
// refuses an identity field the payload does not carry, so a typo in either
// literal is a staging that fails at runtime rather than at compile time.
//
// Deliberately not replayscope's offerDealField, which happens to hold the same
// text: that one names a URL path segment on the offer routes, and binding an
// approval payload's shape to a router's would make either one unmovable.
const (
	closeDateIdentityDeal     = "deal_id"
	closeDateIdentityStanding = "standing_close_date"
)

func (s closeDateStager) HasPendingCorrection(ctx context.Context, dealID ids.UUID) (bool, error) {
	return s.svc.HasPendingKind(ctx, deals.CloseDateCorrectionKind, dealID)
}

// RefusedCloseDate answers the seam's question from the payloads the memory
// stores.
//
// RejectedChangesFor hands back payloads rather than answering a containment
// query because only the caller knows what makes two of its proposals the same
// question. This walks them and lets the probe decide — the judgment is
// RefusalProbe.SameQuestionAs, in the module that owns close dates, because it
// is a fact about corrections rather than about staging.
//
// The read commits before the caller stages, so it can lose a race to a decision
// landing in the gap. That costs one extra offer and never an unasked write —
// what the caller goes on to do is stage, and staging re-checks under its own
// lock.
func (s closeDateStager) RefusedCloseDate(ctx context.Context, dealID ids.UUID, proposed deals.RefusalProbe) (bool, error) {
	refused, err := s.svc.RejectedChangesFor(ctx, deals.CloseDateCorrectionKind, dealID)
	if err != nil {
		return false, err
	}
	for _, payload := range refused {
		earlier, err := deals.UnmarshalCloseDateCorrection(payload)
		if err != nil {
			// A payload an older version of this stager wrote may not decode
			// today, and a decision a human made does not expire because the
			// shape moved. Reading it as "not this one" costs one card they
			// have seen before; failing the sweep would cost every deal's
			// date hygiene tonight.
			continue
		}
		if proposed.SameQuestionAs(earlier) {
			return true, nil
		}
	}
	return false, nil
}

func (s closeDateStager) StageCorrection(ctx context.Context, dealID ids.UUID, targetVersion int64, summary string, proposal deals.CloseDateCorrection) error {
	raw, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("compose: marshal close-date proposal: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return fmt.Errorf("compose: canonicalize close-date proposal: %w", err)
	}
	// The logical identity is the deal AND the date it currently holds — the two
	// fields that say WHICH correction this is, for the purpose of collapsing a
	// LIVE duplicate.
	//
	// Not the deal alone: a decline is remembered with no expiry, so a deal-only
	// identity lets one "no" bury every future correction on that deal — the rep
	// refuses a date, sets their own three weeks later, that one goes stale in
	// turn, and nobody is ever told. The standing date separates those, being new
	// exactly when the rep has moved it.
	//
	// It cannot carry the whole memory by itself, because the sweep writes its
	// date onto the deal BEFORE staging: what the deal stands at tonight is what
	// was proposed last night, so a refusal recorded then matches nothing now.
	// That is what ensureStaged's own refusal check answers.
	identity, err := json.Marshal(map[string]string{
		closeDateIdentityDeal:     proposal.DealID.String(),
		closeDateIdentityStanding: proposal.StandingCloseDate,
	})
	if err != nil {
		return fmt.Errorf("compose: marshal close-date identity: %w", err)
	}
	_, _, err = s.svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		Identity:       identity,
		TargetType:     approvalTargetDeal,
		TargetID:       dealID,
		TargetVersion:  &targetVersion,
		Summary:        summary,
		JoinPending:    true,
	})
	return err
}

// quietReviewReader adapts the deals module's QuietReviewReader seam: read one
// deal's correspondence as the DEAL'S OWNER, never as the sweep's own system
// principal.
//
// The reason it composes is stored in the approval payload and read later by
// anyone holding deal:update on that deal. A system principal passes
// auth.Require unconditionally and no row scope bounds it, so a name resolved
// under it would be any name in the workspace, frozen into a record no
// read-side gate can re-filter. Resolving the owner's real grants — their
// permissions, their teams, their seat, in ONE snapshot — makes the read no
// wider than the person the card is for.
//
// Every failure here is the same answer: no facts, so the review falls back to
// a reason with no name in it. That covers a deal with no owner (owner_id is
// nullable and ON DELETE SET NULL), an owner who has been suspended, archived
// or removed (EffectiveAuthority answers ErrNotFound — absence of authority is
// denial, never empty permission), and an owner whose grants do not reach the
// correspondence. A deal's date hygiene must not depend on any of them.
type quietReviewReader struct {
	db *database.DB
	// owner resolves the reading authority. Shared with the overnight drafter,
	// which has the same obligation for the same reason.
	owner dealOwnerAuthority
}

func (r quietReviewReader) ReadForOwner(ctx context.Context, dealID ids.DealID) (deals.QuietFacts, deals.QuietNames, error) {
	ownerCtx, err := r.owner.contextFor(ctx, dealID.UUID)
	if errors.Is(err, errNoDealOwner) {
		// An unowned deal still gets its review; nobody's authority is the
		// right one to read its correspondence under, so it goes unnamed.
		return deals.QuietFacts{}, nil, nil
	}
	if err != nil {
		return deals.QuietFacts{}, nil, err
	}
	var facts deals.QuietFacts
	var names deals.QuietNames
	err = r.db.Tx(ownerCtx, func(tx pgx.Tx) error {
		var readErr error
		if facts, readErr = deals.ReadQuietFacts(ownerCtx, tx, dealID); readErr != nil {
			return readErr
		}
		names, readErr = r.nameCounterparties(ownerCtx, tx, facts)
		// An owner who may not read people still gets the dates. The two
		// answers are different sizes — WHEN the silence started is on the
		// deal's own correspondence, WHO it was with belongs to the person
		// record — and collapsing the first into the second's refusal throws
		// away a fact the reader is entitled to.
		if errors.Is(readErr, apperrors.ErrPermissionDenied) {
			names = deals.QuietNames{}
			return nil
		}
		return readErr
	})
	if err != nil {
		return deals.QuietFacts{}, nil, err
	}
	return facts, names, nil
}

// nameCounterparties resolves both sides' people in ONE call, so a deal whose
// two directions share a contact costs one read rather than two. It runs inside
// the owner's transaction, so a person the owner may not see simply has no
// entry and the reason says "the contact".
func (r quietReviewReader) nameCounterparties(ctx context.Context, tx pgx.Tx, facts deals.QuietFacts) (deals.QuietNames, error) {
	var persons []ids.PersonID
	for _, side := range []*deals.QuietSide{facts.LastInbound, facts.LastOutbound} {
		if side != nil && !side.PersonID.IsZero() {
			persons = append(persons, ids.From[ids.PersonKind](side.PersonID))
		}
	}
	if len(persons) == 0 {
		return deals.QuietNames{}, nil
	}
	found, err := people.NewStore(r.db).PersonNamesTx(ctx, tx, persons)
	if err != nil {
		return nil, err
	}
	return deals.QuietNames(found), nil
}

// NewCloseDateCorrector assembles the nightly close-date corrector for
// the worker process role.
func NewCloseDateCorrector(pool *pgxpool.Pool, log *slog.Logger) *deals.CloseDateCorrector {
	db := InstallationDB(pool)
	return deals.NewCloseDateCorrector(db,
		closeDateStager{svc: approvals.NewService(db)},
		quietReviewReader{db: db, owner: dealOwnerAuthority{db: db, users: identity.NewServiceFor(db)}},
		log, installseam.Deals())
}

// closeDateConfirmEffect executes an approved close-date confirmation:
// redeem-then-execute like every 🟡 executor, then apply the confirmed
// (possibly human-edited) date through the deals store — the same
// RBAC-gated, INV-CLOSE-PAST-validating update a direct edit takes. It
// runs as the deciding human: confirming the date IS their write, and
// their update also clears the provisional flag.
func closeDateConfirmEffect(svc *approvals.Service, store *deals.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		version, pinned, err := svc.Redeem(ctx, approvalID, deals.CloseDateCorrectionKind, diffHash)
		if err != nil {
			return err
		}
		correction, err := deals.UnmarshalCloseDateCorrection(proposedChange)
		if err != nil {
			return err
		}
		confirmed, err := time.Parse(time.DateOnly, correction.ExpectedCloseDate)
		if err != nil {
			return fmt.Errorf("compose: confirmed close date: %w", err)
		}
		// Redemption validated the pin in ITS transaction and committed; this
		// write opens another. Carrying the pin into the update puts the
		// version compare inside the transaction that actually moves the date,
		// so a deal edited between the two loses to the compare rather than
		// silently taking a date the approver never saw.
		update := deals.UpdateDealInput{ExpectedClose: &confirmed}
		if pinned {
			update.IfVersion = &version
		}
		_, err = store.UpdateDeal(ctx, correction.DealID, update)
		return err
	}
}
