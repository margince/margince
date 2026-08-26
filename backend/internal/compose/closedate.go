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

	"github.com/gradionhq/margince/backend/internal/compose/installseam"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/diffhash"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// closeDateStager adapts the approvals service onto the deals module's
// CorrectionStager seam.
type closeDateStager struct {
	svc *approvals.Service
}

func (s closeDateStager) HasPendingCorrection(ctx context.Context, dealID ids.UUID) (bool, error) {
	return s.svc.HasPendingKind(ctx, deals.CloseDateCorrectionKind, dealID)
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
	_, err = s.svc.Stage(ctx, approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     approvalTargetDeal,
		TargetID:       dealID,
		TargetVersion:  &targetVersion,
		Summary:        summary,
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
	db    *database.DB
	users *identity.Service
}

func (r quietReviewReader) ReadForOwner(ctx context.Context, dealID ids.DealID) (deals.QuietFacts, deals.QuietNames, error) {
	owner, err := r.dealOwner(ctx, dealID)
	if err != nil {
		return deals.QuietFacts{}, nil, err
	}
	if owner.IsZero() {
		// An unowned deal still gets its review; nobody's authority is the
		// right one to read its correspondence under, so it goes unnamed.
		return deals.QuietFacts{}, nil, nil
	}
	ownerCtx, err := r.asOwner(ctx, owner)
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

// dealOwner reads who the deal belongs to. It runs under the caller's own
// principal — the sweep's — because owner_id is what CHOOSES the reading
// authority, and a read that needed the authority to find it could not start.
func (r quietReviewReader) dealOwner(ctx context.Context, dealID ids.DealID) (ids.UUID, error) {
	var owner *ids.UUID
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT owner_id FROM deal WHERE id = $1`, dealID).Scan(&owner)
	})
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: deal %s owner: %w", dealID, err)
	}
	if owner == nil {
		return ids.Nil, nil
	}
	return *owner, nil
}

// asOwner binds the deal owner as the acting principal.
//
// EffectiveAuthority reads the grants and the seat as ONE snapshot: composed
// from separate reads they can describe an authority the owner never held —
// permissions from before a role change with a seat from after. SeatType is
// carried rather than omitted because its zero value fails closed to read-only,
// which would silently narrow a read that is supposed to mirror the owner's.
func (r quietReviewReader) asOwner(ctx context.Context, owner ids.UUID) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: quiet review outside a bound workspace")
	}
	rbac, seat, err := r.users.EffectiveAuthority(ctx, wsID, owner)
	if err != nil {
		return nil, fmt.Errorf("compose: deal owner %s authority: %w", owner, err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          principal.HumanIDPrefix + owner.String(),
		UserID:      owner,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	}), nil
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
		quietReviewReader{db: db, users: identity.NewServiceFor(db)},
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
