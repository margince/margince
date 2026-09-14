// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Store) linkEmploymentEpisode(ctx context.Context, tx pgx.Tx, contact ids.ContactID, company ids.CompanyID, e employmentEvidence, capturedBy string) (*ids.UUID, string, error) {
	args := []any{contact}
	contactPos := len(args)
	args = append(args, company)
	companyPos := len(args)
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT %s FROM relationship WHERE contact_id=$%d AND company_id=$%d AND kind='employment' ORDER BY id`, relationshipColumns, contactPos, companyPos), args...)
	if err != nil {
		return nil, "", err
	}
	relationships, err := scanRelationships(rows)
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	_, started := preciseEmploymentDate(e.Started)
	_, ended := preciseEmploymentDate(e.Ended)
	matched, state := matchEmploymentEpisode(relationships, e, started, ended)
	if state != "" {
		return matched, state, nil
	}
	if !e.validDateRange() {
		return nil, "needs_review", nil
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
	return storekit.EmitEvent(ctx, tx, auditID, contact.UUID, relationshipUpdatedPayload(contactEntity, map[string]any{eventKeyDelta: map[string]any{employmentImportField: state}}))
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
		return nil, "needs_review"
	}
	return nil, ""
}

func undatedCurrentEmployment(r relationshipRow) bool {
	return r.ArchivedAt == nil && r.EndedAt == nil && (r.EmploymentStatus == nil || *r.EmploymentStatus == employmentCurrent)
}

func compatibleCurrentHistory(r relationshipRow, sameRole bool, started, ended *time.Time) bool {
	return sameRole && (started == nil || r.StartedAt == nil) && ended == nil && undatedCurrentEmployment(r)
}
