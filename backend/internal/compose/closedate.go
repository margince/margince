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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeDatePolicy answers the deals module's CorrectionPolicy seam from the
// approvals module's autonomy table.
//
// Bound to the DEAL'S OWNER, never to whoever the sweep is running as: the
// setting is one rep's answer about their own deals, and the sweep is a system
// pass with no seat of its own.
type closeDatePolicy struct {
	svc   *approvals.Service
	owner dealOwnerAuthority
}

// CorrectsWithoutAsking reports whether this owner has left close-date hygiene
// to the sweep.
//
// Compared against ModeAuto rather than "not manual", because there is a third
// rung: a rep on `veto` has asked to see a change before it lands, and reading
// the setting as a two-way switch would take that as consent.
//
// An owner who no longer holds a seat — suspended, archived, removed — answers
// FALSE. Their deals still exist and still drift, but nobody has said the sweep
// may write them unasked, and a missing authority is not consent.
func (p closeDatePolicy) CorrectsWithoutAsking(ctx context.Context, owner ids.UUID) (bool, error) {
	if owner == ids.Nil {
		// An unowned deal has nobody to ask and nobody to leave alone. The
		// sweep corrects it: a date nobody maintains is exactly the one that
		// goes stale, and there is no rep whose morning it would surprise.
		return true, nil
	}
	ownerCtx, err := p.owner.asOwner(ctx, owner)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	// The CHOICE, not the mode alone. AutoApplyMode reports 'manual' both for a
	// rep who asked to be asked and for one who has never seen the setting, and
	// this is the one caller whose default is not manual — so folding the two
	// together would read silence as a refusal and leave every deal in the
	// pipeline on a date nobody maintains.
	choice, err := p.svc.AutonomyChoiceFor(ownerCtx, deals.CloseDateCorrectionKind)
	if err != nil {
		return false, err
	}
	if !choice.Chosen {
		// Nobody has decided. The sweep corrects and reports itself on the
		// morning receipt with a way back, which is the honest version of the
		// card it replaced: that card wrote the date FIRST and then asked a
		// question whose answer changed nothing.
		return true, nil
	}
	// Compared against auto explicitly rather than as "not manual": a third
	// stored rung, veto, exists, and reading it as consent would write
	// unattended for the one rep who asked hardest not to be written for.
	return choice.Mode == approvals.ModeAuto, nil
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
	owner := dealOwnerAuthority{db: db, users: identity.NewServiceFor(db)}
	return deals.NewCloseDateCorrector(db,
		closeDatePolicy{svc: approvals.NewService(db), owner: owner},
		quietReviewReader{db: db, owner: owner},
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
		// A reversal since this card was staged has already answered its
		// question, and this door is the unattended one: close_date_correction
		// is auto-applied for a rep who has turned autonomy on, so a confirm
		// redeemed after an Undo would silently put back what the Undo removed.
		blocked, err := store.ReversalBlocksConfirming(ctx, correction, &confirmed)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("compose: this correction was taken back: %w", apperrors.ErrConflict)
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
