// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanySourceMeeting marks a profile field read from one of the company's
// meetings. It cites the transcript and stores no quote of it, so the meeting's
// own audience decides who reads what was said.
const CompanySourceMeeting = "meeting"

// MeetingProfileField is one field a meeting states about the company.
type MeetingProfileField struct {
	Field      string
	Value      string
	Confidence float64
}

// ApplyMeetingProfileTx writes what one transcript states about a company onto
// its profile fields. It never replaces a value a person or an agent wrote, and
// it returns the fields it wrote. Only a transcript the whole workspace may read
// fills a field: a profile field has the company's readers, and a meeting kept
// to its participants must not reach them through one.
func ApplyMeetingProfileTx(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, transcript ids.ActivityID, fields []MeetingProfileField,
) (map[string]any, error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, "company", companyID.UUID); err != nil {
		return nil, err
	}
	var shared bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity WHERE id = $1
		   AND audience = 'workspace' AND restricted_at IS NULL AND archived_at IS NULL)`, transcript.UUID).Scan(&shared); err != nil {
		return nil, fmt.Errorf("read the transcript's audience: %w", err)
	}
	applied := map[string]any{}
	if !shared {
		return applied, nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	for _, field := range fields {
		value := textbounds.Fit("public.company_profile_field.value", field.Value)
		tag, err := tx.Exec(ctx, upsertCompanyProfileField,
			companyID, field.Field, value, "", nil, field.Confidence, CompanySourceMeeting, by, false, transcript.UUID)
		if err != nil {
			return nil, fmt.Errorf("write profile field %s from a meeting: %w", field.Field, err)
		}
		if tag.RowsAffected() > 0 {
			applied[field.Field] = value
		}
	}
	if len(applied) == 0 {
		return applied, nil
	}
	evidence := map[string]any{auditKeySource: CompanySourceMeeting, auditKeySourceRef: transcript.String(), auditKeyFields: applied}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, companyEntity, companyID.UUID, nil, nil, evidence)
	if err != nil {
		return nil, fmt.Errorf("audit meeting profile: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta:  map[string]any{auditKeyFields: applied},
			auditKeySource: CompanySourceMeeting, auditKeySourceRef: transcript.String(),
		},
	}); err != nil {
		return nil, fmt.Errorf("emit company.updated: %w", err)
	}
	return applied, nil
}

// profileFieldCitationSQL selects a field's cited meeting for this reader: its
// id only when they may read the meeting, its date always. The caller joins
// `activity cited ON cited.id = f.source_activity_id`.
func profileFieldCitationSQL(ctx context.Context, arg func(any) int) (string, error) {
	readable, err := auth.ActivityContentClause(ctx, "cited", arg)
	if err != nil {
		return "", err
	}
	return `CASE WHEN cited.id IS NOT NULL AND (` + readable + `) THEN cited.id END, cited.occurred_at`, nil
}
