// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const introCarrySource = "contact_id=@source OR through_contact_id=@source"
const introCarryContact = "CASE WHEN contact_id=@source THEN @target ELSE contact_id END"
const introCarryThrough = "CASE WHEN through_contact_id=@source THEN @target ELSE through_contact_id END"
const introCarryClosures = `WITH mapped AS (
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

func introCarryArgs(from, to ids.ContactID) pgx.NamedArgs {
	return pgx.NamedArgs{
		"source": from, "target": to, "open": openStatuses(),
		"self_reason": mergedSelfRouteReason, "duplicate_reason": mergedDuplicateReason,
	}
}
