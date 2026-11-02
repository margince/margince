// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Published comparison tokens are pseudonymous subject data, not anonymous text replacements.
func sarThreadDerivations(pkg *SARPackage, authored []ids.UUID) sarSection {
	return sarSection{&pkg.ConversationDerivations, `WITH sources AS (
 SELECT a.id,a.audience,a.thread_key,EXISTS(SELECT 1 FROM capture_thread_invalidation i WHERE i.activity_id=a.id OR i.source_key=a.thread_key
 OR EXISTS(SELECT 1 FROM capture_thread_repair_node n WHERE n.activity_id=a.id AND (n.repair_id=i.repair_id OR EXISTS(
 SELECT 1 FROM capture_thread_repair_node erased WHERE erased.activity_id=i.activity_id AND erased.repair_id=n.repair_id)))) AS pending_cleanup
 FROM activity a WHERE a.id=ANY($1::uuid[]))
 SELECT a.id AS activity_id,a.pending_cleanup,NOT activity_thread_available(a.thread_key) AS publication_pending,
 ` + sarContentDisclosed + ` AS content_disclosed,
 CASE WHEN ` + sarContentDisclosed + ` AND NOT a.pending_cleanup THEN t.own_topic END AS topic_digest,
 CASE WHEN ` + sarContentDisclosed + ` AND NOT a.pending_cleanup THEN t.effective_topic END AS effective_topic_digest,
 CASE WHEN ` + sarContentDisclosed + ` AND NOT a.pending_cleanup THEN t.subject_hash END AS subject_fingerprint,
 CASE WHEN ` + sarContentDisclosed + ` AND NOT a.pending_cleanup THEN t.recognized_keys END AS recognized_project_keys,
 CASE WHEN ` + sarContentDisclosed + ` AND NOT a.pending_cleanup THEN EXISTS(
 SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id AND ci.thread_input_hash<>'') END AS replay_input_known
 FROM sources a LEFT JOIN capture_thread_topic t ON t.activity_id=a.id`, []any{authored}}
}
