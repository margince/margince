// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The per-domain company verdict, read from the COMPANY side.
//
// domaintriage.go reads the ledger by domain, under a lock, because that is
// what deciding needs. This is the other direction and a different question:
// which domains were triaged into THIS company, and what each of them
// concluded. It takes no lock and decides nothing — a member is being told why
// a record exists, not asked to change it.
//
// It is the reason the trace's company-triage stage can be answered at all. The
// stage's subject is a domain, and a domain is triaged once for every message
// that ever arrives from it, so the message ladder cannot honestly carry the
// answer: it would report "done" for the message that prompted the triage and
// for the hundredth one after it alike.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyDomainTriage is one domain's triage answer as the company surface
// reads it.
//
// A read shape of its own rather than DomainDisposition, which carries what the
// WRITE path needs — the lock's admission, the owner the company insert will
// use, the retry budget — and none of what a reader is owed. Two shapes because
// they are two questions; the ledger is one.
type CompanyDomainTriage struct {
	Domain string
	// Status is the ledger's own verdict vocabulary (DomainPending,
	// DomainCompany, DomainNoSite and the two that name no company).
	Status string
	// Source is what produced the verdict, "" while it is still pending.
	Source string
	// PendingReason says why an open question is still open, "" when it is not
	// open or when nothing more specific was recorded.
	PendingReason string
	// DecidedAt is when the row last moved. For a settled domain that is when
	// it was answered; for a pending one it is when it was last looked at,
	// which is what tells a member whether anything is happening.
	DecidedAt time.Time
}

// DomainTriageForCompany lists the domains triaged into one company.
//
// Ordered by domain so two reads of an unchanged company render identically.
// A company nothing triaged into answers an empty list rather than an error:
// a company somebody typed in by hand is not a defect, and the surface says so
// rather than reporting a gap.
func (s *Store) DomainTriageForCompany(ctx context.Context, id ids.CompanyID) ([]CompanyDomainTriage, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []CompanyDomainTriage
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The company's own gate, so a company outside the caller's row scope
		// is existence-hidden exactly as GetCompany hides it — never an empty
		// list, which would say this company had no domains rather than that it
		// is not this reader's.
		if err := ensureCompanyReadable(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT domain, status, COALESCE(source, ''), COALESCE(pending_reason, ''), updated_at
			  FROM company_domain_disposition
			 WHERE company_id = $1
			 ORDER BY domain`, id)
		if err != nil {
			return fmt.Errorf("list the company's domain triage: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row CompanyDomainTriage
			if err := rows.Scan(&row.Domain, &row.Status, &row.Source,
				&row.PendingReason, &row.DecidedAt); err != nil {
				return fmt.Errorf("scan a domain triage row: %w", err)
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
