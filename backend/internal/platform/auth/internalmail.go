// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ActivityExternalEvidenceClause excludes operational colleague mail from customer evidence.
func ActivityExternalEvidenceClause(alias string) string {
	return ActivityAvailableClause(alias) + " AND " + correspondence.ExternalSQL(alias)
}

func internalMailAccessClause(p principal.Principal, alias string, arg func(any) int) string {
	if p.UserID.IsZero() {
		return alias + ".internal_mail_authorization IS NULL"
	}
	reader := arg(p.UserID)
	return fmt.Sprintf(`(CASE WHEN %[1]s.internal_mail_authorization IS NULL THEN true ELSE EXISTS (
  SELECT 1 FROM app_user internal_reader WHERE internal_reader.id = $%[2]d
  AND %[3]s AND %[4]s) END)`, alias, reader, workforce.LiveHumanSQL("internal_reader"), internalMailReaderSQL(alias))
}

func internalMailReaderSQL(alias string) string {
	return fmt.Sprintf(`(%[2]s OR (%[1]s.audience = 'selected' AND EXISTS (
  SELECT 1 FROM activity_audience_member internal_grant WHERE internal_grant.activity_id = %[1]s.id
   AND internal_grant.subject_type = 'user' AND internal_grant.subject_id IN (%[3]s))))`,
		alias, activityAttendanceForMemberSQL(alias, memberIdentityExpressionSQL("internal_reader.id")), memberIdentityExpressionSQL("internal_reader.id"))
}

// EnsureActivityRetentionTargetTx authorizes a controller action without granting message discovery or content.
func EnsureActivityRetentionTargetTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := Require(ctx, "retention_policy", principal.ActionUpdate); err != nil {
		return err
	}
	actor, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if !UnboundedFor(actor, "activity") {
		return EnsureActivityVisible(ctx, tx, id)
	}
	return ensureActivity(ctx, tx, id, func(context.Context, string, func(any) int) (string, error) { return ActivityAvailableClause("a"), nil }, false)
}
