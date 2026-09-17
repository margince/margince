// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The open domain questions belonging to ONE colleague's mail.
//
// The admin list beside this file (domainadmissionlist.go) answers "what has
// this installation decided, and what is still open" for an operator looking at
// capture posture. This answers a different question for a different reader:
// which of these are MINE to answer. A domain's question is opened by a
// message, and the mailbox owner that message reached is recorded on the row —
// so the question has an addressee, and asking it of everybody is how it came
// to be asked of nobody.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DomainQuestion is one undecided domain as its owner sees it.
type DomainQuestion struct {
	Domain string
	// Reason is the machine's grounds for stopping, already in prose. It comes
	// from the same CASE the admin list renders, so the two surfaces cannot
	// come to describe one withholding differently.
	Reason  string
	AskedAt time.Time
}

// OpenDomainQuestionsForOwner lists the undecided domains whose mail belongs to
// the calling human, oldest question first.
//
// OLDEST FIRST, unlike the admin list's newest-first. That list is a log
// somebody scans for what just happened; this is a backlog somebody works
// through, and the question that has waited longest is the one most likely to
// be holding a company out of the CRM.
//
// Read-gated on companies, like the admin list: what this exposes is that a
// company is missing and why. A caller with no human behind it gets NOTHING
// rather than everything — a connector or an agent has no mail of its own, so
// there is no set of questions that is "theirs", and answering with every open
// question would turn one colleague's backlog into the shared pile this exists
// to replace.
func (s *Store) OpenDomainQuestionsForOwner(ctx context.Context) ([]DomainQuestion, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return nil, err
	}
	owner := actingHuman(ctx)
	if owner == nil {
		// REFUSED, not empty. A connector or an agent has no mail of its own, so
		// there is no set of questions that is theirs — and the lane's contract
		// says such a caller is named in `lanes_omitted` rather than told they
		// have nothing waiting. Answering with an empty list would report a
		// cleared backlog to a caller that was never eligible for one.
		return nil, apperrors.ErrPermissionDenied
	}
	var out []DomainQuestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A domain this reader has already discarded is ANSWERED, even though
		// the disposition row still says pending: the answer was recorded as a
		// capture exclusion rather than as a company verdict, because "not for
		// me" is a statement about one colleague's mail and not about what the
		// domain IS. Without this the discard verb writes its rule and the same
		// question returns on the next read, which is the whole feature failing
		// quietly.
		//
		// Scoped the way the capture sink scopes it (sinkexclusion.go): a
		// workspace rule binds every reader, a user rule binds only its own.
		//
		// EQUALITY ALONE, where the sink also matches subdomains, and the
		// difference is in the left-hand side rather than in the rule. The sink
		// compares the domain read off a mail header, so `mail.acme.com`
		// reaches it unfolded and a rule naming `acme.com` has to cover it.
		// This column is always the registrable domain — ensure folds it
		// through freemail.Hostname before the question is ever opened — and an
		// exclusion value is registrable too, since ValidOwnDomain refuses a
		// public suffix. So the subdomain arm could match nothing here, and
		// carrying it would be a line no case can reach.
		rows, err := tx.Query(ctx, `
			SELECT domain, `+domainPendingReason+`, COALESCE(admission_at, updated_at)
			  FROM company_domain_disposition d
			 WHERE d.pending_reason IS NOT NULL AND d.owner_id = $1
			   AND NOT EXISTS (
			         SELECT 1 FROM capture_exclusion e
			          WHERE e.kind = 'domain' AND e.value = d.domain
			            AND (e.scope = 'workspace' OR e.user_id = $1))
			 ORDER BY COALESCE(admission_at, updated_at) ASC`, *owner)
		if err != nil {
			return fmt.Errorf("contacts: listing open domain questions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var q DomainQuestion
			if err := rows.Scan(&q.Domain, &q.Reason, &q.AskedAt); err != nil {
				return fmt.Errorf("contacts: reading an open domain question: %w", err)
			}
			out = append(out, q)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
