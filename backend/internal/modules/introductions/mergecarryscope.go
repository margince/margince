// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	introCarrySource   = "contact_id=@source OR through_contact_id=@source"
	introCarryContact  = "CASE WHEN contact_id=@source THEN @target ELSE contact_id END"
	introCarryThrough  = "CASE WHEN through_contact_id=@source THEN @target ELSE through_contact_id END"
	introCarryClosures = `WITH mapped AS (
 SELECT id, status, version, requested_at, introducer_user_id AS colleague,
 ` + introCarryContact + ` AS about, ` + introCarryThrough + ` AS via
 FROM intro_request WHERE (` + introCarrySource + `) AND status=ANY(@open) AND archived_at IS NULL
), collisions AS (
 SELECT m.id, k.id AS witness FROM mapped m JOIN intro_request k
 ON k.contact_id=m.about AND k.introducer_user_id=m.colleague
 AND k.through_contact_id IS NOT DISTINCT FROM m.via
 WHERE k.status=ANY(@open) AND k.archived_at IS NULL
 AND k.contact_id<>@source AND k.through_contact_id IS DISTINCT FROM @source
 UNION
 SELECT m.id, o.id AS witness FROM mapped m JOIN mapped o
 ON o.about=m.about AND o.colleague=m.colleague AND o.via IS NOT DISTINCT FROM m.via
 WHERE o.about<>o.via AND (o.requested_at,o.id)<(m.requested_at,m.id)
), losers AS (
 SELECT m.id,m.status,m.version,m.about,m.about=m.via AS self_route FROM mapped m
 WHERE m.about=m.via OR EXISTS (SELECT 1 FROM collisions c WHERE c.id=m.id)
)`
)

// introCarryRecordedLosers names the recorded introductions a merge would put
// twice on one pair of colleagues: the unique index on the pair refuses the
// re-home otherwise. The older row stands, whichever contact it was on.
const introCarryRecordedLosers = `
 SELECT CASE WHEN (s.requested_at,s.id)<(t.requested_at,t.id) THEN t.id ELSE s.id END AS id
 FROM intro_request s JOIN intro_request t
 ON t.contact_id=@target AND s.contact_id=@source
 AND t.introducer_user_id=s.introducer_user_id AND t.requester_user_id=s.requester_user_id
 WHERE s.origin='recorded' AND t.origin='recorded' AND s.archived_at IS NULL AND t.archived_at IS NULL`

func introCarryArgs(from, to ids.ContactID) pgx.NamedArgs {
	return pgx.NamedArgs{
		"source": from, "target": to, "open": openStatuses(),
		"self_reason": mergedSelfRouteReason, "duplicate_reason": mergedDuplicateReason,
		"recorded_reason": mergedRecordedDuplicateReason,
	}
}
