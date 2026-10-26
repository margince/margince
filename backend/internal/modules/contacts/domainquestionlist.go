// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DomainQuestionLimit bounds the attention projection; the review route remains paged.
const DomainQuestionLimit = 100

type DomainQuestion struct {
	ID         ids.UUID
	Domain     string
	Reason     string
	ReasonCode string
	AskedAt    time.Time
	EvidenceAt *time.Time
	Deferred   bool
	Forward    *DomainForwardEvidence
	// Note is the site triage's own sentence on a vendor question, so the
	// reader sees why the machine called the domain a vendor.
	Note string
}

func (s *Store) OpenDomainQuestionsForOwner(ctx context.Context) ([]DomainQuestion, error) {
	rows, _, err := s.listReaderDomainQuestions(ctx, "", DomainQuestionLimit, false)
	return rows, err
}

// DomainQuestionsForOwner includes deferred questions so readers can restore them.
func (s *Store) DomainQuestionsForOwner(ctx context.Context, cursor string, limit int) ([]DomainQuestion, storekit.Page, error) {
	return s.listReaderDomainQuestions(ctx, cursor, limit, true)
}

func (s *Store) listReaderDomainQuestions(ctx context.Context, cursor string, limit int, includeDeferred bool) ([]DomainQuestion, storekit.Page, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	owner := actingHuman(ctx)
	if owner == nil {
		return nil, storekit.Page{}, apperrors.ErrPermissionDenied
	}
	limit = storekit.ClampLimit(&limit)
	var out []DomainQuestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		bind := func(value any) int { args = append(args, value); return len(args) }
		arg := func(value any) string { return fmt.Sprintf("$%d", bind(value)) }
		ownerArg := arg(*owner) + "::uuid"
		identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, bind)
		if err != nil {
			return err
		}
		// One instant for the whole read, so the decider and the freshness
		// below agree on which mail is inside the horizon.
		asOf := s.clock()
		open, err := openQuestionForReaderSQL(ctx, "d.domain", ownerArg, identities, asOf, bind)
		if err != nil {
			return err
		}
		// The question's freshness is the newest mail the decider reads, not a
		// reader row. The sole direct recipient may hold no row at all.
		latest := `(SELECT max(question_mail.occurred_at) FROM activity question_mail WHERE ` +
			vendorMailSQL("question_mail", "d.domain", asOf, bind) + `)`
		deferred := `EXISTS (SELECT 1 FROM worklist_done wd WHERE wd.reader_id = ` + ownerArg + ` AND wd.source = 'domain_question' AND wd.row_id = d.domain AND (` + latest + ` IS NULL OR ` + latest + ` <= wd.done_at))`
		where := `d.pending_reason <> ` + arg(PendingReservedDomain) + ` AND ` + open + `
	  AND NOT EXISTS (SELECT 1 FROM capture_exclusion e WHERE e.kind = 'domain' AND e.value = d.domain
	   AND (e.scope = 'workspace' OR e.user_id = ` + ownerArg + `))`
		if !includeDeferred {
			where += ` AND NOT ` + deferred
		}
		if cursor != "" {
			after, err := storekit.DecodeCursor(cursor)
			if err != nil {
				return err
			}
			where += ` AND (d.created_at, d.id) > (` + arg(after.CreatedAt) + `, ` + arg(after.ID) + `)`
		}
		query := `SELECT d.id, d.domain, ` + domainPendingReason + `, d.pending_reason,
	  d.created_at, ` + latest + `, COALESCE(d.evidence, ''), ` + deferred + `
	  FROM company_domain_disposition d WHERE ` + where + `
	  ORDER BY d.created_at, d.id LIMIT ` + arg(limit+1)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q DomainQuestion
			var evidence string
			if err := rows.Scan(&q.ID, &q.Domain, &q.Reason, &q.ReasonCode, &q.AskedAt, &q.EvidenceAt, &evidence, &q.Deferred); err != nil {
				return err
			}
			if q.ReasonCode == PendingVendorVerdict {
				q.Note = evidence
			}
			if q.ReasonCode == PendingForwardedIdentity {
				q.Forward = &DomainForwardEvidence{}
				if err := json.Unmarshal([]byte(evidence), q.Forward); err != nil {
					slog.WarnContext(ctx, "domain forwarding evidence could not be decoded", "domain", q.Domain, "err", err)
					q.Forward = nil
				}
			}
			out = append(out, q)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	page := storekit.Page{HasMore: len(out) > limit}
	if page.HasMore {
		out = out[:limit]
		last := out[len(out)-1]
		page.NextCursor, err = storekit.EncodeCursor(last.AskedAt, last.ID)
	}
	return out, page, err
}
