// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The partner extension (ADR-0032): a company promoted to a
// first-class partner. Identity is never duplicated — partner is a
// one-to-one extension row, and upserting it flips the company's
// classification; the company's own .updated event carries the change.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// livePartnerSQL renders "this company's partner programme is live" — a row
// that has not been retired — qualified by the caller's alias, or unqualified
// when there is none. Shared, because a reader and a guard that spelled this
// differently would disagree about which companies are partners.
func livePartnerSQL(alias string) string {
	if alias == "" {
		return "archived_at IS NULL"
	}
	return alias + ".archived_at IS NULL"
}

// visibleCompanySQL renders "this is a company the caller may read" — live,
// and inside their row scope — for the aliased `company` row a partner read
// hangs off. Shared, because the list correlates it as an EXISTS while the
// batch marker joins it, and two spellings of what a visible company means
// would let one of them mark an account the other hides.
func visibleCompanySQL(ctx context.Context, alias string, arg func(any) int) (string, error) {
	scope, err := scopeOrAllRows(ctx, "company", alias, arg)
	if err != nil {
		return "", fmt.Errorf("contacts: scoping the partner's company: %w", err)
	}
	return fmt.Sprintf("%s.archived_at IS NULL AND (%s)", alias, scope), nil
}

const partnerColumns = `company_id, cert_status, partner_kinds, margin_tier,
	certified_staff, retention_rate, relationship_stage, next_step, next_step_due_at,
	served_segments, partner_fit_score, partner_fit_score_computed,
	partner_fit_override_reason, relationship_health::text, last_contact_at,
	version, created_at, updated_at, archived_at,
	activated_at, activation_basis, activated_by, activation_lead_id, activation_deal_id`

type partnerRow struct {
	CompanyID         ids.CompanyID
	CertStatus        string
	PartnerKinds      []string
	MarginTier        *string
	CertifiedStaff    int16
	RetentionRate     *int16
	RelationshipStage string
	NextStep          *string
	NextStepDueAt     *time.Time
	ServedSegments    []string
	// The ADR-0053 Commercial Judgement pair (formulas §17): a non-nil
	// FitOverrideReason marks FitScore human-set; the machine value is then
	// retained in FitScoreComputed instead of overwriting FitScore.
	FitScore           *int16
	FitScoreComputed   *int16
	FitOverrideReason  *string
	RelationshipHealth *string // numeric(3,2) rendered ::text — no float touches it
	LastContactAt      *time.Time
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
	Activation         partnerActivation
}

// partnerActivation is the stamp that lets a partner stand on a stage meaning
// active. The lead and deal are evidence references, not foreign keys.
type partnerActivation struct {
	At     *time.Time
	Basis  *string
	By     *string
	LeadID *ids.UUID
	DealID *ids.UUID
}

func scanPartner(r pgx.Row) (partnerRow, error) {
	var p partnerRow
	err := r.Scan(&p.CompanyID, &p.CertStatus, &p.PartnerKinds, &p.MarginTier,
		&p.CertifiedStaff, &p.RetentionRate, &p.RelationshipStage, &p.NextStep, &p.NextStepDueAt,
		&p.ServedSegments, &p.FitScore, &p.FitScoreComputed, &p.FitOverrideReason,
		&p.RelationshipHealth, &p.LastContactAt, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
		&p.Activation.At, &p.Activation.Basis, &p.Activation.By, &p.Activation.LeadID, &p.Activation.DealID)
	return p, err
}

type UpsertPartnerInput struct {
	CompanyID ids.CompanyID
	// PartnerKinds replaces the kinds stored. Nil reads PartnerRole, the
	// deprecated single kind, as a list of one.
	PartnerKinds      *[]string
	PartnerRole       string
	CertStatus        *string
	MarginTier        *string
	CertifiedStaff    *int16
	RetentionRate     *int16
	RelationshipStage *string
	NextStep          *string
	NextStepDueAt     *time.Time
	ServedSegments    *[]string
	// Cleared names the fields the caller sent as an explicit null; a nil
	// pointer above means "leave it alone" and cannot say "clear it".
	Cleared []string
	// The fit-override pair is store-seam only: the contract exposes
	// partner_fit_score/…_override_reason read-only, so no handler maps
	// them — the invariant is pinned here for the future fit engine.
	FitScore *int16
	// FitOverrideReason is tri-state, mirroring the lead-score pair: nil =
	// no override change; non-nil empty = the explicit CLEAR gesture;
	// non-nil non-empty = the written reason for a human fit score.
	FitOverrideReason *string
	IfVersion         *int64
	// MustCreate makes the write a set-up. It refuses a company with a live
	// program. It starts the program from the column defaults, even when it
	// revives an archived row that kept a tier, metrics, segments or a fit override.
	MustCreate bool
}

// PartnerFitOverrideReasonRequiredError rejects a human partner-fit score
// with no written reason — the Commercial Judgement rule (formulas §17,
// ADR-0053): an override is auditable or it does not happen.
type PartnerFitOverrideReasonRequiredError struct{}

func (e *PartnerFitOverrideReasonRequiredError) Error() string {
	return "a partner_fit_score override requires a non-empty partner_fit_override_reason"
}

// FieldFault refuses an unexplained partner-fit override: the reason is the audit trail.
func (e *PartnerFitOverrideReasonRequiredError) FieldFault() (field, code, message string) {
	return "partner_fit_override_reason", codeRequired, e.Error()
}

// partnerFitState is the override pair's column triple as one value.
type partnerFitState struct {
	Score          *int16
	Computed       *int16
	OverrideReason *string
}

// applyPartnerFitOverride folds the §17 sticky-override rules — the exact
// sibling of the lead-score applyScoreOverride: setting a score demands a
// written reason and retains the machine value in Computed the first
// time; an explicit empty reason clears the override and Score reverts to
// the retained machine value (recompute, once the fit engine exists,
// resumes from there); a non-empty reason with no score amends the note
// on an override already in force.
func applyPartnerFitOverride(current partnerFitState, score *int16, reason *string) (partnerFitState, error) {
	overrideInForce := current.OverrideReason != nil

	switch {
	case score != nil:
		trimmed := ""
		if reason != nil {
			trimmed = strings.TrimSpace(*reason)
		}
		if trimmed == "" {
			return partnerFitState{}, &PartnerFitOverrideReasonRequiredError{}
		}
		next := partnerFitState{Score: score, OverrideReason: &trimmed, Computed: current.Computed}
		if !overrideInForce {
			next.Computed = current.Score
		}
		return next, nil

	case reason != nil && strings.TrimSpace(*reason) == "":
		if !overrideInForce {
			return current, nil // no override to clear — a no-op
		}
		return partnerFitState{Score: current.Computed}, nil

	case reason != nil:
		if !overrideInForce {
			return partnerFitState{}, &PartnerFitOverrideReasonRequiredError{} // a reason without a score sets nothing
		}
		trimmed := strings.TrimSpace(*reason)
		return partnerFitState{Score: current.Score, Computed: current.Computed, OverrideReason: &trimmed}, nil
	}
	return current, nil
}

// UpsertPartner writes a company's partner terms, promoting the company to a
// partner if it is not one yet. Promotion is a company mutation, so the write
// takes the company grant; setting the margin tier also takes the partner one.
func (s *Store) UpsertPartner(ctx context.Context, in UpsertPartnerInput) (crmcontracts.Partner, error) {
	if err := requirePartnerWriteGrant(ctx); err != nil {
		return crmcontracts.Partner{}, err
	}
	if err := refuseUnclearablePartnerFields(in.Cleared); err != nil {
		return crmcontracts.Partner{}, err
	}
	if err := refuseBlankServedSegments(in.ServedSegments); err != nil {
		return crmcontracts.Partner{}, err
	}
	kinds, err := partnerKindsOf(in)
	if err != nil {
		return crmcontracts.Partner{}, err
	}
	in.PartnerKinds = &kinds
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Partner{}, err
	}
	var out crmcontracts.Partner
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The company reference comes from the client, so it is probed for write
		// authority. A `read` share must not let its holder reclassify the company.
		if err := auth.EnsureWritable(ctx, tx, "company", in.CompanyID.UUID); err != nil {
			return err
		}
		// The row lock makes the version pre-read and the upsert below one
		// race-free unit; partner is keyed by its company, so the company
		// row is the serialization point.
		if _, err := storekit.LockRow(ctx, tx, "company", in.CompanyID.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		cleared, err := clearsTheCallerMayMake(ctx, tx, in.Cleared)
		if err != nil {
			return err
		}
		in.Cleared = cleared
		if err := requireMarginTierGrant(ctx, in); err != nil {
			return err
		}
		if in.MustCreate {
			if err := checkPartnerSetUp(ctx, tx, in.CompanyID); err != nil {
				return err
			}
			in = asFreshProgram(in)
		}
		stage, err := resolvePartnerStage(ctx, tx, in)
		if err != nil {
			return err
		}
		// Before the upsert and under the lock: the statement coalesces every
		// absent field onto its current value, so what the request actually
		// moved is only knowable by comparing against the standing row.
		before, err := readPartnerImage(ctx, tx, in.CompanyID)
		if err != nil {
			return err
		}
		fit, err := resolvePartnerFit(ctx, tx, in)
		if err != nil {
			return err
		}
		written, err := writePartner(ctx, tx, partnerUpsert{in: in, fit: fit, stage: stage, capturedBy: capturedBy}, before)
		if err != nil {
			return err
		}
		if written.landed {
			auditID, err := storekit.Audit(ctx, tx, "update", "company", in.CompanyID.UUID, written.before, written.after)
			if err != nil {
				return err
			}
			if err := storekit.EmitEvent(ctx, tx, auditID, in.CompanyID.UUID, crmcontracts.PublicEventCompanyUpdated{
				ChangedFields: map[string]any{
					eventKeyDelta: map[string]any{eventKeyPartner: map[string]any{
						"role": legacyPartnerRole(kinds), "kinds": kinds, "cert_status": written.row.CertStatus,
					}},
				},
			}); err != nil {
				return err
			}
		}
		// The echo is a read: the writer's own role masks it like any other.
		out, err = wirePartnerForCaller(ctx, tx, written.row)
		return err
	})
	return out, err
}

// resolvePartnerFit pre-reads the current row under the company lock — one
// read feeds both the version guard and the fit-override fold (ErrNoRows
// means this upsert is the promotion) — and folds the §17 override rules.
func resolvePartnerFit(ctx context.Context, tx pgx.Tx, in UpsertPartnerInput) (partnerFitState, error) {
	var currentVersion *int64
	var current partnerFitState
	err := tx.QueryRow(ctx,
		`SELECT version, partner_fit_score, partner_fit_score_computed, partner_fit_override_reason
		 FROM partner WHERE company_id = $1`, in.CompanyID).
		Scan(&currentVersion, &current.Score, &current.Computed, &current.OverrideReason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return partnerFitState{}, err
	}
	if in.IfVersion != nil && currentVersion != nil && *currentVersion != *in.IfVersion {
		return partnerFitState{}, apperrors.ErrVersionSkew
	}
	if in.MustCreate {
		current = partnerFitState{}
	}
	return applyPartnerFitOverride(current, in.FitScore, in.FitOverrideReason)
}

// partnerUpsert is one resolved upsert: the request, the fit fold and the stage
// the row lands on.
type partnerUpsert struct {
	in         UpsertPartnerInput
	fit        partnerFitState
	stage      partnerStageWrite
	capturedBy string
}

// upsertPartnerRow lands the promotion-or-update: absent lifecycle fields
// keep their current value (coalesce), while the fit triple is always
// written as resolved — a cleared override must land its NULLs.
func upsertPartnerRow(ctx context.Context, tx pgx.Tx, up partnerUpsert) (partnerRow, error) {
	in, fit := up.in, up.fit
	// served_segments keeps its current value only when the field is
	// absent; pgx maps a nil any to SQL NULL for the coalesce.
	var segments any
	if in.ServedSegments != nil {
		segments = *in.ServedSegments
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO partner (company_id, partner_kinds, cert_status, margin_tier,
		                     certified_staff, retention_rate, relationship_stage, next_step,
		                     next_step_due_at, served_segments, partner_fit_score,
		                     partner_fit_score_computed, partner_fit_override_reason, source, captured_by)
		VALUES ($1, $2, coalesce($3, 'applied'), $4, coalesce($5, 0), $6,
		        $16, $9, $10, $11, $12, $13, $14, 'manual', $7)
		ON CONFLICT (company_id) DO UPDATE SET
		  partner_kinds = EXCLUDED.partner_kinds,
		  cert_status = coalesce($3, partner.cert_status),
		  margin_tier = CASE WHEN 'margin_tier' = ANY($15) THEN NULL ELSE coalesce($4, partner.margin_tier) END,
		  certified_staff = CASE WHEN 'gate_metrics' = ANY($15) THEN 0 ELSE coalesce($5, partner.certified_staff) END,
		  retention_rate = CASE WHEN 'gate_metrics' = ANY($15) THEN NULL ELSE coalesce($6, partner.retention_rate) END,
		  relationship_stage = coalesce($8, partner.relationship_stage),
		  next_step = CASE WHEN 'next_step' = ANY($15) THEN NULL ELSE coalesce($9, partner.next_step) END,
		  next_step_due_at = CASE WHEN 'next_step_due_at' = ANY($15) THEN NULL ELSE coalesce($10, partner.next_step_due_at) END,
		  served_segments = CASE WHEN 'served_segments' = ANY($15) THEN NULL ELSE coalesce($11, partner.served_segments) END,
		  partner_fit_score = $12,
		  partner_fit_score_computed = $13,
		  partner_fit_override_reason = $14,
		  archived_at = NULL
		RETURNING `+partnerColumns,
		in.CompanyID, *in.PartnerKinds, in.CertStatus, in.MarginTier,
		in.CertifiedStaff, in.RetentionRate, up.capturedBy,
		up.stage.set, in.NextStep, in.NextStepDueAt, segments,
		fit.Score, fit.Computed, fit.OverrideReason, in.Cleared, up.stage.insert)
	return scanPartner(row)
}

// GetPartner reads the partner row a company plays, if it plays one.
func (s *Store) GetPartner(ctx context.Context, companyID ids.CompanyID) (crmcontracts.Partner, error) {
	if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
		return crmcontracts.Partner{}, err
	}
	// Partner rows are company-derived data.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.Partner{}, err
	}
	var out crmcontracts.Partner
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		row, err := scanPartner(tx.QueryRow(ctx,
			`SELECT `+partnerColumns+` FROM partner WHERE company_id = $1 AND `+livePartnerSQL(""),
			companyID))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound // the company is not a partner
		}
		if err != nil {
			return err
		}
		out, err = wirePartnerForCaller(ctx, tx, row)
		return err
	})
	return out, err
}

// MarginTierOf answers what commercial tier a partner company is on, or
// nil when it is not a partner or its tier was never set.
//
// A narrow accessor rather than a use of GetPartner because the caller needs
// exactly one field and no company read: the commission accrual prices a
// win it was handed, it does not open the partner's record. It is still gated
// on `partner` read — a tier is commercial terms, and the fact that the only
// caller today runs as a system actor is not a reason to leave the door open
// for the next one.
//
// Answering nil rather than an error for "not a partner" is what lets the
// accrual treat an unpriced win as an ordinary outcome instead of a failure to
// retry forever.
//
// It is deliberately UNMASKED, and the one read of the tier that is. A mask
// applies where a row meets the wire; this value is priced with, never printed,
// and withholding it here would accrue the wrong money rather than disclose
// none. The entry the accrual writes is masked on its own read.
func (s *Store) MarginTierOf(ctx context.Context, companyID ids.CompanyID) (*string, error) {
	if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
		return nil, err
	}
	var tier *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT margin_tier FROM partner WHERE company_id = $1 AND `+livePartnerSQL(""),
			companyID).Scan(&tier)
		if errors.Is(err, pgx.ErrNoRows) {
			tier = nil
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read partner margin tier: %w", err)
	}
	return tier, nil
}
