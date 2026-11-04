// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which mailboxes the nightly input check could read, for the installation and
// for each deal's owner.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sourceMail names the mailbox in coverage rows; the assurance store's CHECK
// constraint owns the vocabulary.
const sourceMail = "mail"

// mailProviders are the capture connectors that carry mail. gcal is calendar
// and the chat providers are messaging. A healthy WhatsApp says nothing about
// whether anybody read the mailbox a close date is checked against.
var mailProviders = []string{providerGmail, "imap", providerGraph, "offline_demo"}

// mailCoverage grades the mailbox by CONNECTOR health, not by mail volume.
//
// Mail volume would conflate two facts. A quiet mailbox on a healthy connector
// would read as stale, because nothing arrived. A broken connector under
// recent mail would read as checked. The sync checkpoint is the fact this
// state claims: when the connector last read the source.
//
// This is the run's grade over every mailbox in the installation. A deal's
// own silence is judged on its owner's mailboxes (mailCoverageByOwner), so one
// broken mailbox does not blind the check for every deal.
func mailCoverage(ctx context.Context, tx pgx.Tx, now time.Time) assurance.SourceCoverage {
	var counts mailCounts
	if err := tx.QueryRow(ctx, `
		SELECT `+mailCountsSQL+`
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.provider = ANY($1) AND c.archived_at IS NULL`,
		mailProviders).Scan(&counts.live, &counts.reauth, &counts.unsynced, &counts.oldestSuccess); err != nil {
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageUnavailable}
	}
	return counts.grade(now)
}

// mailCountsSQL is the aggregate both mail gradings read.
//
// It fails closed over every live connection that nobody disconnected. min()
// ignores NULL, so a never-synced broken mailbox next to a healthy one would
// otherwise vanish from the aggregate. The run would then claim checked over a
// source it never read.
const mailCountsSQL = `count(*) FILTER (WHERE c.status <> 'disconnected'),
		       count(*) FILTER (WHERE c.status = 'reauth_required'),
		       count(*) FILTER (WHERE c.status <> 'disconnected' AND s.last_success_at IS NULL),
		       min(s.last_success_at) FILTER (WHERE c.status <> 'disconnected')`

// mailCounts is the state of a set of mail connections: all of them for the
// run, or one user's for a deal they own.
type mailCounts struct {
	live, reauth, unsynced int
	oldestSuccess          *time.Time
}

// grade turns the counts into one coverage answer.
func (m mailCounts) grade(now time.Time) assurance.SourceCoverage {
	switch {
	case m.live == 0:
		// Never configured, or every mailbox disconnected by its user. Either
		// way there is nothing to fix, only something to decide. Both route to
		// other contacts than a broken connector does.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageNotConnected}
	case m.reauth > 0:
		// Somebody must re-grant access; until then part of the mailbox is
		// unreadable for a permission reason, not a technical one.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoveragePermissionLimited}
	case m.unsynced > 0 || m.oldestSuccess == nil:
		// At least one live mailbox has never been read. One healthy sibling
		// does not make the source checked: the claim covers all of them.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageUnavailable}
	case now.Sub(*m.oldestSuccess) > staleAfter:
		// The slowest mailbox decides. "Mail was checked" over three mailboxes
		// means all three, so the oldest checkpoint grades the source. It also
		// caps the checked-through date below.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageStale}
	default:
		return assurance.SourceCoverage{
			Source: sourceMail, State: assurance.CoverageChecked, CheckedThrough: m.oldestSuccess,
		}
	}
}

// mailCoverageByOwner grades each user's own live mailboxes, keyed by user.
// A user with no live mailbox has no entry.
//
// It is what a deal's silence is judged on (dealMailCovered). A broken mailbox
// of a user the deal's mail never reached does not make that deal
// coverage_unknown.
func mailCoverageByOwner(ctx context.Context, tx pgx.Tx, now time.Time) (map[ids.UUID]assurance.SourceCoverage, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.user_id, `+mailCountsSQL+`
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.provider = ANY($1) AND c.archived_at IS NULL
		GROUP BY c.user_id
		HAVING count(*) FILTER (WHERE c.status <> 'disconnected') > 0`, mailProviders)
	if err != nil {
		return nil, fmt.Errorf("compose: reading each owner's mailboxes: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID]assurance.SourceCoverage{}
	for rows.Next() {
		var user ids.UUID
		var counts mailCounts
		if err := rows.Scan(&user, &counts.live, &counts.reauth, &counts.unsynced, &counts.oldestSuccess); err != nil {
			return nil, fmt.Errorf("compose: reading each owner's mailboxes: %w", err)
		}
		out[user] = counts.grade(now)
	}
	return out, rows.Err()
}

// dealMailboxesSQL is the mail connections that delivered a deal's mail, as
// text. A connection a capture_import row names in delivered_by reads
// "<user>:<provider>:<generation>". An import that names none reads "<user>".
// Only mail every member may read counts, as for customer progress, so every
// reader computes the same list. The viability fingerprint carries it, so a
// mailbox that delivers the deal's mail later makes the receipt stale.
func dealMailboxesSQL(deal string) string {
	return `ARRAY(SELECT DISTINCT ci.user_id::text || coalesce(':' || dm.mailbox, '')
		          FROM capture_import ci
		          JOIN activity ma ON ma.id = ci.activity_id
		          JOIN activity_link mal ON mal.activity_id = ma.id
		          LEFT JOIN LATERAL unnest(ci.delivered_by) AS dm(mailbox) ON true
		         WHERE mal.deal_id = ` + deal + `.id
		           AND ma.kind = 'email'
		           AND ma.archived_at IS NULL
		           AND ` + visibleToAllSQL("ma") + `
		         ORDER BY 1)`
}

// mailConnection names one mail connection: the user and the provider. A user
// holds one connection per provider.
type mailConnection struct {
	user     ids.UUID
	provider string
}

// connectionCoverage is one connection's grade and the generation of the
// account it is bound to now.
type connectionCoverage struct {
	generation string
	coverage   assurance.SourceCoverage
}

// mailCoverageByConnection grades each mail connection on its own, with the
// same grade the run uses.
func mailCoverageByConnection(ctx context.Context, tx pgx.Tx, now time.Time) (map[mailConnection]connectionCoverage, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.user_id, c.provider, c.generation::text, `+mailCountsSQL+`
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.provider = ANY($1) AND c.archived_at IS NULL
		GROUP BY c.id, c.user_id, c.provider, c.generation`, mailProviders)
	if err != nil {
		return nil, fmt.Errorf("compose: reading each mail connection: %w", err)
	}
	defer rows.Close()
	out := map[mailConnection]connectionCoverage{}
	for rows.Next() {
		var key mailConnection
		var generation string
		var counts mailCounts
		if err := rows.Scan(&key.user, &key.provider, &generation,
			&counts.live, &counts.reauth, &counts.unsynced, &counts.oldestSuccess); err != nil {
			return nil, fmt.Errorf("compose: reading each mail connection: %w", err)
		}
		out[key] = connectionCoverage{generation: generation, coverage: counts.grade(now)}
	}
	return out, rows.Err()
}

// dealMailCovered answers, for one deal, whether the mail a silence would be
// read from was read on time.
//
// The deal's own mailboxes decide, whatever the run's grade says. A run grade
// of checked proves only that the connected mailboxes are healthy. It does not
// prove that the deal's mail reaches one of them. The deal's mailboxes are the
// owner's live connections, and each connection that delivered its mail. A
// delivering connection that is gone, bound to another account, or not read on
// time leaves the deal unknown. So does an owner with no live mailbox. An
// import that names no connection is judged on that user's live connections.
// A deal with no owner and no captured mail has no mailboxes of its own and
// keeps the run's grade.
//
// mailboxes are dealMailboxesSQL's entries. One that does not parse fails
// closed.
func dealMailCovered(
	runCovered bool, byOwner map[ids.UUID]assurance.SourceCoverage, byConnection map[mailConnection]connectionCoverage,
) func(owner *ids.UUID, mailboxes []string) bool {
	userChecked := func(user ids.UUID) bool {
		own, ok := byOwner[user]
		return ok && own.State == assurance.CoverageChecked
	}
	return func(owner *ids.UUID, mailboxes []string) bool {
		if owner == nil && len(mailboxes) == 0 {
			return runCovered
		}
		if owner != nil && !userChecked(*owner) {
			return false
		}
		for _, entry := range mailboxes {
			rawUser, mailbox, named := strings.Cut(entry, ":")
			user, err := ids.Parse(rawUser)
			if err != nil {
				return false
			}
			if !named {
				if !userChecked(user) {
					return false
				}
				continue
			}
			provider, generation, ok := strings.Cut(mailbox, ":")
			if !ok {
				return false
			}
			conn, live := byConnection[mailConnection{user: user, provider: provider}]
			if !live || conn.generation != generation || conn.coverage.State != assurance.CoverageChecked {
				return false
			}
		}
		return true
	}
}
