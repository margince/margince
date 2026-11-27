// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// interactionEdgeFacts are the folded columns of graph_interaction_edge, as the
// row comparison that lets an unchanged fold skip its write.
var interactionEdgeFacts = []string{
	"last_at", "last_precision", "last_inbound_at", "last_outbound_at", "last_met_at",
	"count_90d", "in_count_90d", "out_count_90d", "sent_count_90d",
	"late_count_90d", "late_out_count_90d", "count_total",
}

// foldPairsSQL is the upsert that refolds named pairs, built once per mode so
// the columns it reads can be derived from the statement itself.
func foldPairsSQL(advanceSnapshot bool) string {
	window := fmt.Sprintf("now() - interval '%d days'", relstrength.WindowDays)
	return `WITH target AS (
		    SELECT DISTINCT user_id, contact_id
		      FROM unnest($1::uuid[], $2::uuid[]) AS t(user_id, contact_id)
		),
		counted AS (
		    SELECT t.user_id AS edge_user, t.contact_id AS edge_contact, ` + relstrength.FoldColumnsSQL("a") + `, ` + graphMet + ` AS met, ` + graphMemberSent + ` AS sent
		      FROM target t
		      JOIN activity_participant up
		        ON up.user_id = t.user_id AND up.role IN ` + interactionRoles + `
		      JOIN activity_participant pp
		        ON pp.activity_id = up.activity_id
		       AND pp.contact_id = t.contact_id AND pp.role IN ` + interactionRoles + `
		       AND ` + liveContactSQL("pp.contact_id") + `
		      JOIN activity a
		        ON a.id = up.activity_id AND a.archived_at IS NULL` + audienceWorkspaceOnly + `
		   AND ` + graphCountedActivity + ` AND ` + auth.ParticipantStandsSQL("up", "a") + `
		),
		folded AS (
		    SELECT a.edge_user AS user_id, a.edge_contact AS contact_id, ` + graphFoldColumns(window) + `
		      FROM (` + graphEngagedRows + `) a
		     GROUP BY a.edge_user, a.edge_contact
		)
		INSERT INTO graph_interaction_edge AS e
		    (user_id, contact_id, last_at, last_precision, last_inbound_at, last_outbound_at, last_met_at,
		     count_90d, in_count_90d, out_count_90d, sent_count_90d, late_count_90d, late_out_count_90d, count_total, read_snapshot)
		SELECT f.user_id, f.contact_id, f.last_at, f.last_precision, f.last_inbound_at, f.last_outbound_at, f.last_met_at,
		       f.count_90d, f.in_90d, f.out_90d, f.sent_90d, f.late_90d, f.late_out_90d, f.count_total, pg_current_snapshot()
		  FROM folded f
		 ORDER BY f.user_id, f.contact_id
		ON CONFLICT (user_id, contact_id) DO UPDATE SET
		    ` + edgeUpsertSet(interactionEdgeFacts) + unchangedEdgeGuard(advanceSnapshot, interactionEdgeFacts) + `
        RETURNING e.user_id, e.contact_id`
}

// pruneInteractionEdgesSQL deletes the named pairs that have no qualifying
// interaction left.
func pruneInteractionEdgesSQL() string {
	return `DELETE FROM graph_interaction_edge e
		 USING unnest($1::uuid[], $2::uuid[]) AS t(user_id, contact_id)
		 WHERE e.user_id = t.user_id AND e.contact_id = t.contact_id
		   AND NOT EXISTS (
		       SELECT 1
		         FROM activity_participant up
		         JOIN activity_participant pp ON pp.activity_id = up.activity_id
		         JOIN activity a ON a.id = up.activity_id AND a.archived_at IS NULL` + audienceWorkspaceOnly + `
		   AND ` + graphCountedActivity + ` AND ` + auth.ParticipantStandsSQL("up", "a") + `
		        WHERE up.user_id = t.user_id AND up.role IN ` + interactionRoles + `
		          AND pp.contact_id = t.contact_id AND pp.role IN ` + interactionRoles + `
		          AND ` + liveContactSQL("pp.contact_id") + `)`
}
