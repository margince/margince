// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanySourceMeeting marks a profile field read from one of the company's
// meetings. It cites the transcript and stores no quote of it, so the meeting's
// own audience decides who reads what was said.
const CompanySourceMeeting = "meeting"

// meetingTranscriptSharedSQL is a transcript, alias t, that may fill a company
// field. Every reader may open it, and it is not restricted, archived or redacted.
const meetingTranscriptSharedSQL = `t.audience = 'workspace' AND t.restricted_at IS NULL AND t.archived_at IS NULL
	AND cardinality(t.redacted_fields) = 0`

// RetractStaleMeetingProfilesTx removes up to limit company fields whose
// transcript may no longer fill them: narrowed, restricted, archived or redacted
// since. One statement tests and deletes, so a transcript that qualifies again
// meanwhile keeps its fields. It returns how many it removed. It sweeps every
// company and so is the system's alone: no caller's write right covers that.
func RetractStaleMeetingProfilesTx(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `WITH picked AS MATERIALIZED (
		  SELECT s.id FROM company_profile_field s JOIN activity t ON t.id = s.source_activity_id
		   WHERE s.source = 'meeting' AND NOT (`+meetingTranscriptSharedSQL+`)
		   ORDER BY s.id LIMIT $1 FOR UPDATE OF s SKIP LOCKED)
		DELETE FROM company_profile_field f USING activity t
		WHERE f.id IN (SELECT id FROM picked)
		  AND f.source = 'meeting' AND t.id = f.source_activity_id AND NOT (`+meetingTranscriptSharedSQL+`)
		RETURNING f.company_id, f.field, f.value`, limit)
	if err != nil {
		return 0, fmt.Errorf("remove the company fields whose meeting no longer stands: %w", err)
	}
	changed := map[ids.CompanyID]*meetingFieldImages{}
	removed := 0
	for rows.Next() {
		var company ids.UUID
		var field string
		var was *string
		if err := rows.Scan(&company, &field, &was); err != nil {
			rows.Close()
			return 0, fmt.Errorf("remove the company fields whose meeting no longer stands: %w", err)
		}
		changedOf(changed, ids.From[ids.CompanyKind](company)).note(field, was, nil)
		removed++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("remove the company fields whose meeting no longer stands: %w", err)
	}
	for company, images := range changed {
		if err := recordMeetingProfile(ctx, tx, company, images); err != nil {
			return 0, err
		}
	}
	return removed, nil
}

// MeetingProfileField is one field a meeting states about the company.
type MeetingProfileField struct {
	Field      string
	Value      string
	Confidence float64
}

// ApplyMeetingProfileTx writes the fields one transcript states. It removes
// what an earlier reading of it wrote and this one no longer states. A user's
// or agent's value and our own company are never written. Only a transcript the
// whole workspace may read fills a field, since the company's readers see it.
// A company the caller may not change keeps its fields as they are.
func ApplyMeetingProfileTx(
	ctx context.Context, tx pgx.Tx, transcript ids.ActivityID, stated map[ids.CompanyID][]MeetingProfileField,
) error {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return err
	}
	var shared bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity t WHERE t.id = $1 AND `+meetingTranscriptSharedSQL+`)`,
		transcript.UUID).Scan(&shared); err != nil {
		return fmt.Errorf("read the transcript's audience: %w", err)
	}
	if !shared {
		stated = nil
	}
	stated, err := withoutAnchorCompany(ctx, tx, stated)
	if err != nil {
		return err
	}
	changeable, err := meetingChangeableCompanies(ctx, tx, transcript, stated)
	if err != nil {
		return err
	}
	maps.DeleteFunc(stated, func(company ids.CompanyID, _ []MeetingProfileField) bool {
		return !slices.Contains(changeable, company.UUID)
	})
	changed := map[ids.CompanyID]*meetingFieldImages{}
	if err := retractUnstatedMeetingFields(ctx, tx, transcript, stated, changeable, changed); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	for company, fields := range stated {
		if err := writeMeetingFieldsTx(ctx, tx, company, transcript, by, fields, changedOf(changed, company)); err != nil {
			return err
		}
	}
	for company, fields := range changed {
		if err := recordMeetingProfile(ctx, tx, company, fields); err != nil {
			return err
		}
	}
	return nil
}

// writeMeetingFieldsTx writes one company's stated fields and notes each it changed.
func writeMeetingFieldsTx(
	ctx context.Context, tx pgx.Tx, company ids.CompanyID, transcript ids.ActivityID, by string,
	fields []MeetingProfileField, images *meetingFieldImages,
) error {
	for _, field := range fields {
		var was *string
		if err := tx.QueryRow(ctx, `SELECT value FROM company_profile_field WHERE company_id = $1 AND field = $2`,
			company, field.Field).Scan(&was); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read profile field %s before a meeting writes it: %w", field.Field, err)
		}
		value := textbounds.Fit("public.company_profile_field.value", field.Value)
		tag, err := tx.Exec(ctx, upsertCompanyProfileField,
			company, field.Field, value, "", nil, field.Confidence, CompanySourceMeeting, by, false, transcript.UUID)
		if err != nil {
			return fmt.Errorf("write profile field %s from a meeting: %w", field.Field, err)
		}
		if tag.RowsAffected() > 0 {
			images.note(field.Field, was, &value)
		}
	}
	return nil
}

// withoutAnchorCompany drops our own company: a meeting describes the other side.
func withoutAnchorCompany(
	ctx context.Context, tx pgx.Tx, stated map[ids.CompanyID][]MeetingProfileField,
) (map[ids.CompanyID][]MeetingProfileField, error) {
	out := map[ids.CompanyID][]MeetingProfileField{}
	for company, fields := range stated {
		var anchor bool
		if err := tx.QueryRow(ctx, `SELECT is_anchor FROM company WHERE id = $1`, company).Scan(&anchor); err != nil {
			return nil, fmt.Errorf("read whether company %s is our own: %w", company, err)
		}
		if !anchor {
			out[company] = fields
		}
	}
	return out, nil
}

// meetingChangeableCompanies asks, before anything is written, which companies
// this apply may change. One it writes to must be writable and live. One it only
// removes values from must be the caller's to change.
func meetingChangeableCompanies(
	ctx context.Context, tx pgx.Tx, transcript ids.ActivityID, stated map[ids.CompanyID][]MeetingProfileField,
) ([]ids.UUID, error) {
	var touched []ids.UUID
	if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(DISTINCT company_id), '{}') FROM company_profile_field
		 WHERE source = 'meeting' AND source_activity_id = $1`, transcript.UUID).Scan(&touched); err != nil {
		return nil, fmt.Errorf("read the companies a meeting wrote to: %w", err)
	}
	for company := range stated {
		touched = append(touched, company.UUID)
	}
	slices.SortFunc(touched, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	var changeable []ids.UUID
	for _, company := range slices.Compact(touched) {
		_, writes := stated[ids.From[ids.CompanyKind](company)]
		err := ensureMeetingChangeable(ctx, tx, company, writes)
		switch {
		case err == nil:
			changeable = append(changeable, company)
		case !errors.Is(err, apperrors.ErrPermissionDenied) && !errors.Is(err, apperrors.ErrNotFound):
			return nil, err
		}
	}
	return changeable, nil
}

func ensureMeetingChangeable(ctx context.Context, tx pgx.Tx, company ids.UUID, writes bool) error {
	if writes {
		return auth.EnsureWritableLive(ctx, tx, companyEntity, company)
	}
	return auth.EnsureRetractable(ctx, tx, companyEntity, company)
}

// retractUnstatedMeetingFields removes the values this transcript wrote on a
// changeable company that the new reading does not state. It notes each
// removal in changed (nil value).
func retractUnstatedMeetingFields(
	ctx context.Context, tx pgx.Tx, transcript ids.ActivityID, stated map[ids.CompanyID][]MeetingProfileField,
	changeable []ids.UUID, changed map[ids.CompanyID]*meetingFieldImages,
) error {
	var keepCompanies []ids.UUID
	var keepFields []string
	for company, fields := range stated {
		for _, field := range fields {
			keepCompanies, keepFields = append(keepCompanies, company.UUID), append(keepFields, field.Field)
		}
	}
	rows, err := tx.Query(ctx, `
		DELETE FROM company_profile_field f
		 WHERE f.source = 'meeting' AND f.source_activity_id = $1 AND f.company_id = ANY($4::uuid[])
		   AND (f.company_id, f.field) NOT IN (SELECT * FROM unnest($2::uuid[], $3::text[]))
		RETURNING f.company_id, f.field, f.value`, transcript.UUID, keepCompanies, keepFields, changeable)
	if err != nil {
		return fmt.Errorf("remove the fields a meeting no longer states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var company ids.UUID
		var field string
		var was *string
		if err := rows.Scan(&company, &field, &was); err != nil {
			return fmt.Errorf("remove the fields a meeting no longer states: %w", err)
		}
		changedOf(changed, ids.From[ids.CompanyKind](company)).note(field, was, nil)
	}
	return rows.Err()
}

// meetingFieldImages are one company's profile fields before and after a
// meeting's write, keyed by field; nil is a field with no row.
type meetingFieldImages struct {
	before, after map[string]any
}

func (m *meetingFieldImages) note(field string, before, after *string) {
	m.before[field], m.after[field] = before, after
}

func changedOf(changed map[ids.CompanyID]*meetingFieldImages, company ids.CompanyID) *meetingFieldImages {
	if changed[company] == nil {
		changed[company] = &meetingFieldImages{before: map[string]any{}, after: map[string]any{}}
	}
	return changed[company]
}

// recordMeetingProfile writes one company's audit row and event. Neither names
// the transcript: an event reaches subscribers the meeting's audience may not.
//
//craft:ignore naked-any the audit evidence seam is jsonb
func recordMeetingProfile(ctx context.Context, tx pgx.Tx, company ids.CompanyID, images *meetingFieldImages) error {
	if len(images.after) == 0 {
		return nil
	}
	fields := images.after
	evidence := map[string]any{auditKeySource: CompanySourceMeeting}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, companyEntity, company.UUID, images.before, images.after, evidence)
	if err != nil {
		return fmt.Errorf("audit meeting profile: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, company.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta:  map[string]any{auditKeyFields: fields},
			auditKeySource: CompanySourceMeeting,
		},
	}); err != nil {
		return fmt.Errorf("emit company.updated: %w", err)
	}
	return nil
}

// profileFieldCitationSQL selects a field's cited meeting for this reader: its
// id only when they may read the meeting, its date and precision always. The caller joins
// `activity cited ON cited.id = f.source_activity_id`.
func profileFieldCitationSQL(ctx context.Context, arg func(any) int) (string, error) {
	readable, err := auth.ActivityContentClause(ctx, "cited", arg)
	if err != nil {
		return "", err
	}
	return `CASE WHEN cited.id IS NOT NULL AND (` + readable + `) THEN cited.id END, cited.occurred_at, cited.occurred_precision`, nil
}
