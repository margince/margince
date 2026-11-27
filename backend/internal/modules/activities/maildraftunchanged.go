// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// draftHoldsContent reports a draft at the expected version that already holds
// what the replace would write. The moment comes from the same
// update_intent_time the replace uses. An agent's draft never holds it, because
// a human's save is what takes the draft over from the agent.
func draftHoldsContent(ctx context.Context, tx pgx.Tx, args pgx.NamedArgs) (bool, error) {
	var holds bool
	err := tx.QueryRow(ctx, `
 SELECT NOT m.agent_drafted
    AND (m.to_addresses, m.cc_addresses, m.bcc_addresses, m.subject, m.body, m.html_body, m.attachment_ids,
         m.communication_context, m.editor_state, m.override_explanation, m.recipient_countries,
         m.scheduled_at, m.scheduled_tz, m.scheduled_wall)
    IS NOT DISTINCT FROM
        (@to::text[], @cc::text[], @bcc::text[], @subject::text, @body::text, @html::text, @attachments::uuid[],
         @context::text, @editor::jsonb, @override_explanation::text, @recipient_countries::jsonb, planned.instant, COALESCE(planned.timezone, @zone), planned.wall_time)
   FROM mail_draft m,
        public.update_intent_time(m.scheduled_wall, m.scheduled_tz, m.scheduled_at, @wall::text::timestamp, @zone, @at) AS planned
  WHERE m.author_id = @author AND m.anchor_type = @anchor_type AND m.anchor_id = @anchor_id AND m.version = @version
    FOR UPDATE OF m`, args).Scan(&holds)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mail draft: comparing the saved content: %w", err)
	}
	return holds, nil
}
