// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func recordDomainQuestionReader(ctx context.Context, tx pgx.Tx, domain string, reader ids.UUID, evidenceAt *time.Time) error {
	if reader == (ids.UUID{}) {
		return nil
	}
	var id ids.UUID
	var previous *time.Time
	var existed bool
	if err := tx.QueryRow(ctx, `SELECT d.id, r.evidence_at, r.reader_id IS NOT NULL
 FROM company_domain_disposition d LEFT JOIN company_domain_question_reader r
 ON r.domain = d.domain AND r.reader_id = @reader WHERE d.domain = @domain`, pgx.NamedArgs{auditKeyDomain: domain, "reader": reader}).Scan(&id, &previous, &existed); err != nil {
		return err
	}
	args := []any{domain, reader, evidenceAt}
	query := `INSERT INTO company_domain_question_reader (domain, reader_id, evidence_at)
 VALUES (` + storekit.Placeholders(args) + `)
 ON CONFLICT (domain, reader_id) DO UPDATE
 SET evidence_at = EXCLUDED.evidence_at
 WHERE EXCLUDED.evidence_at > company_domain_question_reader.evidence_at
 OR (company_domain_question_reader.evidence_at IS NULL AND EXCLUDED.evidence_at IS NOT NULL)`
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("contacts: recording domain question reader: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	after := map[string]any{auditKeyDomain: domain, "reader_id": reader, "evidence_at": evidenceAt}
	if existed {
		before := map[string]any{auditKeyDomain: domain, "reader_id": reader, "evidence_at": previous}
		_, err = storekit.Audit(ctx, tx, "update", "company_domain_question_reader", id, before, after)
	} else {
		_, err = storekit.Audit(ctx, tx, "create", "company_domain_question_reader", id, nil, after)
	}
	return err
}

// OwnsDomainQuestion checks membership independently of the bounded attention page.
func (s *Store) OwnsDomainQuestion(ctx context.Context, domain string) (bool, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return false, err
	}
	reader := actingHuman(ctx)
	if reader == nil {
		return false, apperrors.ErrPermissionDenied
	}
	args := []any{domain, *reader}
	var visible bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
 SELECT 1 FROM company_domain_question_reader r JOIN company_domain_disposition d ON d.domain = r.domain
 WHERE (r.domain, r.reader_id) = (`+storekit.Placeholders(args)+`) AND d.pending_reason IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM capture_exclusion e WHERE e.kind = 'domain' AND e.value = r.domain
 AND (e.scope = 'workspace' OR e.user_id = r.reader_id)))`, args...).Scan(&visible)
	})
	return visible, err
}
