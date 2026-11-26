// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func snapshotPolicyFacts(ctx context.Context, tx pgx.Tx, decision commsauthz.Decision) (commsauthz.Decision, error) {
	if decision.Verdict == commsauthz.VerdictAllow || decision.SubjectID.IsZero() {
		return decision, nil
	}
	stops, err := liveSuppression(ctx, tx, decision.SubjectID.String(), decision.Recipient)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	facts := make([]string, 0, len(stops)+1)
	for _, stop := range stops {
		facts = append(facts, stop.Revision)
	}
	var withdrawals string
	err = tx.QueryRow(ctx, `SELECT coalesce(string_agg(id::text, ',' ORDER BY id),'') FROM consent_event
 WHERE new_state='withdrawn' AND (contact_id=@subject OR lead_id=@subject)`, pgx.StrictNamedArgs{fieldSubject: decision.SubjectID}).Scan(&withdrawals)
	if err != nil {
		return commsauthz.Decision{}, fmt.Errorf("consent: reading the facts behind the message warning: %w", err)
	}
	facts = append(facts, withdrawals)
	slices.Sort(facts)
	decision.Evidence.PolicyRevision = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(facts, ";"))))
	return decision, nil
}
