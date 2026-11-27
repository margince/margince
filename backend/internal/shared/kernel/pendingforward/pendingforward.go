// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package pendingforward names a chat message a workspace member forwarded
// whose original author the provider did not say. It is held for that member
// alone until they file it on a record or discard it, or until it expires.
// Capture writes and lists it and privacy expires it, so the definition lives
// here once.
package pendingforward

import "time"

// Window is how long a pending forward waits for its member before it is
// deleted with its raw row.
const Window = 7 * 24 * time.Hour

// AudienceReason is the hold a message that names no contact is born with,
// and the reason capture and activities already stamp for it. A link to a
// record lifts it.
const AudienceReason = "no_counterparty"

// Predicate is the SQL condition a pending forward satisfies, for the activity
// table under alias. A chat message is always filed with a counterparty. One
// held with none and linked to no record is therefore a forward waiting to be
// filed. The audience gate decides who may read it.
func Predicate(alias string) string {
	return alias + `.kind = 'message' AND ` + alias + `.channel_provider IS NOT NULL
	 AND ` + alias + `.audience = 'participants' AND ` + alias + `.audience_reason = '` + AudienceReason + `'
	 AND ` + alias + `.archived_at IS NULL AND ` + alias + `.restricted_at IS NULL
	 AND NOT EXISTS (SELECT 1 FROM activity_link pl WHERE pl.activity_id = ` + alias + `.id)`
}
