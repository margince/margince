// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) linkEmploymentEpisode(ctx context.Context, tx pgx.Tx, contact ids.ContactID, company ids.CompanyID, e employmentEvidence, capturedBy string, automatic bool) (*ids.UUID, string, error) {
	if err := auth.HoldWritableLive(ctx, tx, contactEntity, contact.UUID); err != nil {
		return nil, "", err
	}
	relationships, err := lockPairEmployments(ctx, tx, contact, company)
	if err != nil {
		return nil, "", err
	}
	_, started := preciseEmploymentDate(e.Started)
	_, ended := preciseEmploymentDate(e.Ended)
	matched, state := matchEmploymentEpisode(relationships, e, started, ended)
	// Current evidence must not revive a link a human ended. Historical or
	// undated roles remain non-current and can coexist in the imported history.
	if automatic && matched == nil && e.Status == employmentCurrent {
		removed, err := pairEndedByHand(ctx, tx, contact, company)
		if err != nil {
			return nil, "", err
		}
		if removed {
			return nil, employmentNeedsReview, nil
		}
	}
	if state, err = s.endOlderUndatedTx(ctx, tx, relationships, started, e, capturedBy, state); err != nil {
		return nil, "", err
	}
	if state != "" {
		return matched, state, nil
	}
	if !e.validDateRange() {
		return nil, employmentNeedsReview, nil
	}
	primary := false
	primaryChoice := &primary
	if e.Status == employmentCurrent {
		primaryChoice = nil
	}
	row, err := writeRelationshipInTx(ctx, tx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &contact, CompanyID: &company,
		Role: &e.Role, IsCurrentPrimary: primaryChoice, StartedAt: started, EndedAt: ended, EmploymentStatus: &e.Status,
		StartedPrecision: employmentPrecision(e.Started), EndedPrecision: employmentPrecision(e.Ended), Source: e.provider,
		Evidence: map[string]any{auditKeyProvider: e.provider, employmentRunField: e.runID, "episode_key": e.key},
	}, capturedBy)
	if err != nil {
		return nil, "", err
	}
	return &row.ID, employmentLinked, nil
}

func sameEmploymentDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

func recordEmploymentOutcome(ctx context.Context, tx pgx.Tx, contact ids.ContactID, e employmentEvidence, state string, company *ids.CompanyID, relationship *ids.UUID) error {
	var previous *string
	var previousCompany, previousRelationship *ids.UUID
	beforeArgs := []any{e.claimID, e.key}
	beforeErr := tx.QueryRow(ctx, storekit.SQLf("SELECT state, company_id, relationship_id FROM provider_employment_resolution WHERE claim_id=$%d AND episode_key=$%d", 1, len(beforeArgs)), beforeArgs...).Scan(&previous, &previousCompany, &previousRelationship)
	if beforeErr != nil && !errors.Is(beforeErr, pgx.ErrNoRows) {
		return beforeErr
	}
	if previousCompany != nil {
		if err := auth.EnsureVisible(ctx, tx, companyEntity, *previousCompany); err != nil {
			return err
		}
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := storekit.SQLf(`INSERT INTO provider_employment_resolution(contact_id,claim_id,episode_key,state,company_id,relationship_id)
 VALUES($%d,$%d,$%d,$%d,$%d,$%d)
 ON CONFLICT(claim_id,episode_key) DO UPDATE SET state=EXCLUDED.state,company_id=EXCLUDED.company_id,
 relationship_id=EXCLUDED.relationship_id,updated_at=now()
 WHERE provider_employment_resolution.state IS DISTINCT FROM EXCLUDED.state
 OR provider_employment_resolution.company_id IS DISTINCT FROM EXCLUDED.company_id
 OR provider_employment_resolution.relationship_id IS DISTINCT FROM EXCLUDED.relationship_id`,
		arg(contact), arg(e.claimID), arg(e.key), arg(state), arg(company), arg(relationship))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if state == employmentLinked && relationship != nil {
		if err := recordApplied(ctx, tx, contact.UUID, e.runID, e.provider, []appliedField{{table: tableRelationship, field: fieldEmployment, rowID: relationship}}); err != nil {
			return err
		}
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", contactEntity, contact.UUID,
		map[string]any{employmentImportField: previous, companyFK: previousCompany, "relationship_id": previousRelationship}, map[string]any{employmentImportField: state, companyFK: company, "relationship_id": relationship},
		map[string]any{auditKeyProvider: e.provider, employmentRunField: e.runID, "episode_key": e.key})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, contact.UUID, relationshipUpdatedPayload(ctx, contactEntity, map[string]any{eventKeyDelta: map[string]any{employmentImportField: state}}))
}

// Each purchase keeps its own support even when the episode was already resolved.
func copyEmploymentOutcome(ctx context.Context, tx pgx.Tx, contact ids.ContactID, e employmentEvidence) error {
	prior, err := employmentImportItem(ctx, tx, contact, e)
	if err != nil {
		return err
	}
	return recordEmploymentOutcome(ctx, tx, contact, e, string(prior.State), idArg[ids.CompanyKind](prior.CompanyId), uuidArg(prior.RelationshipId))
}

func matchEmploymentEpisode(relationships []relationshipRow, e employmentEvidence, started, ended *time.Time) (*ids.UUID, string) {
	competingCurrent := false
	for _, r := range relationships {
		sameRole := r.Role != nil && strings.EqualFold(strings.TrimSpace(*r.Role), e.Role)
		if sameRole && sameEmploymentDate(r.StartedAt, started) && sameEmploymentDate(r.EndedAt, ended) {
			if r.ArchivedAt != nil {
				return &r.ID, actionDismissed
			}
			return &r.ID, employmentLinked
		}
		if compatibleCurrentHistory(r, sameRole, started, ended) {
			return &r.ID, employmentLinked
		}
		if e.Status == employmentCurrent && undatedCurrentEmployment(r) {
			competingCurrent = true
		}
	}
	if competingCurrent {
		return nil, employmentNeedsReview
	}
	return nil, ""
}

func undatedCurrentEmployment(r relationshipRow) bool {
	return r.ArchivedAt == nil && r.EndedAt == nil && (r.EmploymentStatus == nil || *r.EmploymentStatus == employmentCurrent)
}

func compatibleCurrentHistory(r relationshipRow, sameRole bool, started, ended *time.Time) bool {
	return sameRole && (started == nil || r.StartedAt == nil) && ended == nil && undatedCurrentEmployment(r)
}

// endUndatedEmploymentsTx closes each undated current employment as former,
// ending at the start of the newer episode e reports. Each close is an
// ordinary audited relationship edit, so it can be edited back.
func (s *Store) endUndatedEmploymentsTx(
	ctx context.Context, tx pgx.Tx, relationships []relationshipRow, start time.Time, precision *string, e employmentEvidence, capturedBy string,
) error {
	former := employmentFormer
	for _, r := range relationships {
		if !undatedCurrentEmployment(r) {
			continue
		}
		if _, err := s.updateRelationshipInTx(ctx, tx, r.ID, UpdateRelationshipInput{
			EmploymentStatus: &former, EndedAt: &start, EndedPrecision: precision,
			Evidence: map[string]any{auditKeyProvider: e.provider, employmentRunField: e.runID, "superseded_by_episode": e.key, "replacement_claim": e.claimID.String(), "replacement_episode": e.key},
		}, capturedBy); err != nil {
			return err
		}
	}
	return nil
}

// endOlderUndatedTx ends the undated current roles at the company when dated
// evidence would otherwise wait for review. The older role ends where the new
// one starts. It answers the state that remains.
func (s *Store) endOlderUndatedTx(ctx context.Context, tx pgx.Tx, relationships []relationshipRow, started *time.Time, e employmentEvidence, capturedBy, state string) (string, error) {
	if state != employmentNeedsReview || started == nil || !e.validDateRange() || !undatedEndAt(relationships, *started) {
		return state, nil
	}
	mayEnd, err := mayEndUndatedTx(ctx, tx, relationships)
	if err != nil || !mayEnd {
		return state, err
	}
	if err := s.endUndatedEmploymentsTx(ctx, tx, relationships, *started, employmentPrecision(e.Started), e, capturedBy); err != nil {
		return state, err
	}
	return "", nil
}

// lockPairEmployments reads and locks every employment of one contact at one
// company, oldest first.
func lockPairEmployments(ctx context.Context, tx pgx.Tx, contact ids.ContactID, company ids.CompanyID) ([]relationshipRow, error) {
	args := []any{contact}
	contactPos := len(args)
	args = append(args, company)
	companyPos := len(args)
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT %s FROM relationship WHERE contact_id=$%d AND company_id=$%d AND kind='employment' ORDER BY id FOR UPDATE`, relationshipColumns, contactPos, companyPos), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelationships(rows)
}

// pairEndedByHand is whether somebody ended this contact's employment at the
// company, so new evidence for the pair waits for a human.
func pairEndedByHand(ctx context.Context, tx pgx.Tx, contact ids.ContactID, company ids.CompanyID) (bool, error) {
	var removed bool
	err := tx.QueryRow(ctx, "SELECT "+employment.EndedAtCompanySQL("$1::uuid", "$2::uuid"), contact, company).Scan(&removed)
	return removed, err
}

// undatedEndAt reports whether every undated current employment began before
// start, so ending each there keeps its dates in order.
func undatedEndAt(relationships []relationshipRow, start time.Time) bool {
	for _, r := range relationships {
		if undatedCurrentEmployment(r) && r.StartedAt != nil && !r.StartedAt.Before(start) {
			return false
		}
	}
	return true
}

// mayEndUndatedTx reports whether this import may end the undated current
// rows itself. The caller must hold relationship:update, and no user may have
// set any such row's status or dates. Otherwise the pair stays for review.
func mayEndUndatedTx(ctx context.Context, tx pgx.Tx, relationships []relationshipRow) (bool, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	for _, r := range relationships {
		if !undatedCurrentEmployment(r) {
			continue
		}
		var human bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_log decided
			 WHERE decided.entity_type = 'relationship' AND decided.entity_id = $1 AND decided.actor_type IN ('human','agent')
			   AND (decided.after ?| ARRAY['employment_status', 'started_at', 'ended_at']))`, r.ID).Scan(&human); err != nil {
			return false, fmt.Errorf("contacts: reading who set an employment's dates: %w", err)
		}
		if human {
			return false, nil
		}
	}
	return true, nil
}
