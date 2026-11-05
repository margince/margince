// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentEvidenceUnavailable marks retained provenance whose source content was erased.
const CommitmentEvidenceUnavailable = "source_unavailable"

// CommitmentContentRetainedClause excludes held work whose source words were erased.
// The alias is a compile-time SQL identifier supplied by the reader.
func CommitmentContentRetainedClause(alias string) string {
	return fmt.Sprintf("(coalesce(%s.source_system, '') <> '%s' OR %s)", alias, provenance.CommitmentTaskSource, commitmentEvidenceRetained(alias+".source_activity_id", alias+".source_id"))
}

func commitmentEvidenceRetained(source, locator string) string {
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM activity_commitment_admission erased_commitment
 WHERE erased_commitment.source_activity_id = %s
 AND erased_commitment.locator = %s AND erased_commitment.outcome = '%s')`, source, locator, CommitmentEvidenceUnavailable)
}

// CommitmentEvidenceClause keeps a renewal behind the renewing message's current content boundary.
// source and locator are compile-time SQL expressions supplied by the owning reader.
func CommitmentEvidenceClause(ctx context.Context, source, locator string, arg func(any) int) (string, error) {
	actor, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return commitmentEvidenceArm(actor, source, locator, arg), nil
}

func commitmentEvidenceArm(actor principal.Principal, source, locator string, arg func(any) int) string {
	content := activityDiscoverClause(actor, "renewal_source", arg)
	if actor.Type != principal.PrincipalSystem {
		content += " AND " + activityAudienceArm(actor, "renewal_source", arg)
	}
	return commitmentEvidenceRetained(source, locator) + " AND " + fmt.Sprintf(`NOT EXISTS (
 SELECT 1 FROM activity_commitment_admission renewal
 LEFT JOIN activity renewal_source ON renewal_source.id = renewal.renewed_by
 WHERE renewal.source_activity_id = %s AND renewal.locator = %s
 AND renewal.renewed_by IS NOT NULL AND
 (renewal_source.id IS NULL OR renewal_source.restricted_at IS NOT NULL OR NOT (%s)))`, source, locator, content)
}
