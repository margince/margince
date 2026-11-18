// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/kernel/provenance"

// priorOutboundJoin finds OUR newest message before candidate a in the same
// conversation (ourOutboundInThisThread), or nothing. Its body goes to the
// model with a's, so it must be judgeable by a's rule (judgeableAudienceSQL).
func priorOutboundJoin(includePrivate bool) string {
	return `LEFT JOIN LATERAL (
  SELECT prior.id, prior.subject, left(prior.body, $%[4]d) AS body, prior.occurred_at
    ` + priorOutboundFrom(includePrivate) + `) prior ON true`
}

// priorOutboundFrom finds that message for candidate a.
func priorOutboundFrom(includePrivate bool) string {
	return `FROM activity prior
   WHERE a.thread_key IS NOT NULL
     AND a.counterparty_email IS NOT NULL
     AND ` + ourOutboundInThisThread("prior", "a") + `
     AND ` + judgeableAudienceSQL("prior", includePrivate) + `
     AND (prior.occurred_at, prior.id) < (a.occurred_at, a.id)
   ORDER BY prior.occurred_at DESC, prior.id DESC
   LIMIT 1`
}

// priorContextComplete refuses a candidate whose newest prior message of ours
// was not read in full. That message is the context the verdict turns on, and
// skipping to an older one would show the model a stale exchange as current.
// A subquery rather than the join, so it can sit inside the waiting scan.
func priorContextComplete(includePrivate bool) string {
	return `coalesce((SELECT ` + provenance.MailBodyCompleteSQL("prior") + ` ` + priorOutboundFrom(includePrivate) + `), true)`
}
