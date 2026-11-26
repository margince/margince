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
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	served_segments, version, created_at, updated_at, archived_at,
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
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ArchivedAt        *time.Time
	Activation        partnerActivation
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
		&p.ServedSegments, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
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
	Cleared   []string
	IfVersion *int64
	// MustCreate makes the write a set-up. It refuses a company with a live
	// program. It starts the program from the column defaults, even when it
	// revives an archived row that kept a tier, metrics or segments.
	MustCreate bool
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
	nextStep, err := takePartnerNextStep(&in)
	if err != nil {
		return crmcontracts.Partner{}, err
	}
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
		if err := refuseAgentPromotion(ctx, tx, in); err != nil {
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
		if err := checkPartnerVersion(ctx, tx, in); err != nil {
			return err
		}
		written, err := writePartner(ctx, tx, partnerUpsert{in: in, stage: stage, capturedBy: capturedBy}, before)
		if err != nil {
			return err
		}
		if err := recordPartnerWrite(ctx, tx, in.CompanyID, kinds, written); err != nil {
			return err
		}
		if err := s.writePartnerNextStepTx(ctx, tx, nextStep); err != nil {
			return err
		}
		// The echo is a read: the writer's own role masks it like any other.
		out, err = wirePartnerForCaller(ctx, tx, written.row)
		return err
	})
	return out, err
}

// recordPartnerWrite writes the audit row and the company event for a partner
// write that changed something, in the write's own transaction.
func recordPartnerWrite(ctx context.Context, tx pgx.Tx, company ids.CompanyID, kinds []string, written partnerWrite) error {
	if !written.landed {
		return nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "company", company.UUID, written.before, written.after)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, company.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta: map[string]any{eventKeyPartner: map[string]any{
				"role": legacyPartnerRole(kinds), eventKeyPartnerKinds: kinds, "cert_status": written.row.CertStatus,
			}},
		},
	})
}

// checkPartnerVersion reads the version under the company lock. ErrNoRows
// means this upsert is the promotion, which has no version to skew.
func checkPartnerVersion(ctx context.Context, tx pgx.Tx, in UpsertPartnerInput) error {
	if in.IfVersion == nil {
		return nil
	}
	var current int64
	err := tx.QueryRow(ctx, `SELECT version FROM partner WHERE company_id = $1`, in.CompanyID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if current != *in.IfVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// partnerUpsert is one resolved upsert: the request and the stage the row
// lands on.
type partnerUpsert struct {
	in         UpsertPartnerInput
	stage      partnerStageWrite
	capturedBy string
}

// upsertPartnerRow lands the promotion-or-update: absent lifecycle fields
// keep their current value (coalesce).
func upsertPartnerRow(ctx context.Context, tx pgx.Tx, up partnerUpsert) (partnerRow, error) {
	in := up.in
	// served_segments keeps its current value only when the field is
	// absent; pgx maps a nil any to SQL NULL for the coalesce.
	var segments any
	if in.ServedSegments != nil {
		segments = *in.ServedSegments
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO partner (company_id, partner_kinds, cert_status, margin_tier,
		                     certified_staff, retention_rate, relationship_stage, next_step,
		                     next_step_due_at, served_segments, source, captured_by)
		VALUES ($1, $2, coalesce($3, 'applied'), $4, coalesce($5, 0), $6,
		        $13, $9, $10, $11, '`+provenance.RecordSourceManual+`', $7)
		ON CONFLICT (company_id) DO UPDATE SET
		  partner_kinds = EXCLUDED.partner_kinds,
		  cert_status = coalesce($3, partner.cert_status),
		  margin_tier = CASE WHEN 'margin_tier' = ANY($12) THEN NULL ELSE coalesce($4, partner.margin_tier) END,
		  certified_staff = CASE WHEN 'gate_metrics' = ANY($12) THEN 0 ELSE coalesce($5, partner.certified_staff) END,
		  retention_rate = CASE WHEN 'gate_metrics' = ANY($12) THEN NULL ELSE coalesce($6, partner.retention_rate) END,
		  relationship_stage = coalesce($8, partner.relationship_stage),
		  next_step = CASE WHEN 'next_step' = ANY($12) THEN NULL ELSE coalesce($9, partner.next_step) END,
		  next_step_due_at = CASE WHEN 'next_step_due_at' = ANY($12) THEN NULL ELSE coalesce($10, partner.next_step_due_at) END,
		  served_segments = CASE WHEN 'served_segments' = ANY($12) THEN NULL ELSE coalesce($11, partner.served_segments) END,
		  archived_at = NULL
		RETURNING `+partnerColumns,
		in.CompanyID, *in.PartnerKinds, in.CertStatus, in.MarginTier,
		in.CertifiedStaff, in.RetentionRate, up.capturedBy,
		up.stage.set, in.NextStep, in.NextStepDueAt, segments,
		in.Cleared, up.stage.insert)
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
