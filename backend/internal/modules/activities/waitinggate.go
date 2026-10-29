// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A waiting statement gates its messages, and the replies that may lift a
// snooze on them, once as waitingReadable over waitingCandidates. Postgres
// plans each copy of the content gate, and a pooled connection keeps that
// memory.
const (
	waitingCandidates = "waiting_candidates"
	waitingReadable   = "waiting_readable"
)

// waitingCandidatesSQL is the mail a waiting statement may report: inbound,
// begun, the reader's to answer and inside the horizon. Its terms are ones
// waitingRepliesSQL also tests, so it drops no row the statement reports.
// reply_wakes marks a message a reply may bring back, the only one whose
// thread the statement reads.
func waitingCandidatesSQL(asOf, reader, responsibility string, horizon int) string {
	return `SELECT a.id, a.thread_key, a.kind, a.channel_provider,
	       ` + readerSetAsideSQL(reader, "mine.reopen_on IS DISTINCT FROM 'reply'") + ` AS reply_wakes
	  FROM activity a
	 WHERE a.kind IN ('email', 'message') AND a.direction = 'inbound' AND a.archived_at IS NULL
	   AND ` + occurredday.BegunSQL("a", asOf+"::timestamptz") + `
	   AND (` + responsibility + `)
	   AND ` + withinWaitingHorizonSQL(asOf, fmt.Sprint(horizon))
}

// waitingWithSQL is the `WITH` head that names candidates. It gates them, and
// the live rows on their threads, once.
func waitingWithSQL(ctx context.Context, candidates string, arg func(any) int) (string, error) {
	readable, err := auth.ContentReadableActivitiesSQL(ctx, waitingNeedSQL(), arg)
	if err != nil {
		return "", err
	}
	return "WITH " + waitingCandidates + " AS MATERIALIZED (" + candidates + "),\n" +
		waitingReadable + " AS MATERIALIZED (" + readable + ")\n", nil
}

func waitingReadableSQL(alias string) string {
	return alias + ".id IN (SELECT id FROM " + waitingReadable + ")"
}

// waitingNeedSQL names each candidate and, for one a reply may bring back, the
// inbound rows on its thread: the snooze-lift test reads nothing else. Gating
// every thread of every candidate cost more than the copies it saved.
func waitingNeedSQL() string {
	return `SELECT c.id FROM ` + waitingCandidates + ` c
	 UNION SELECT t.id FROM ` + waitingCandidates + ` c JOIN activity t ON ` + correspondence.ThreadSQL("t", "c") + `
	   AND t.restricted_at IS NULL AND t.direction = 'inbound'
	 WHERE c.reply_wakes`
}

// threadNeedSQL names each of candidates and the live rows on its thread. A
// statement gates that set once when its later-message and snooze-lift tests
// read the same thread; both match by correspondence.ThreadSQL. candidates
// names a relation with an activity's id, thread_key, kind and channel_provider.
func threadNeedSQL(candidates string) string {
	return `SELECT c.id FROM ` + candidates + ` c
	 UNION SELECT t.id FROM ` + candidates + ` c JOIN activity t ON ` + correspondence.ThreadSQL("t", "c") + ` AND t.restricted_at IS NULL`
}
