// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwnsDomainQuestion checks membership independently of the bounded attention page.
func (s *Store) OwnsDomainQuestion(ctx context.Context, domain string) (bool, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return false, err
	}
	reader := actingHuman(ctx)
	if reader == nil {
		return false, apperrors.ErrPermissionDenied
	}
	var visible bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		visible, err = s.holdsOpenQuestionTx(ctx, tx, domain, *reader)
		return err
	})
	return visible, err
}

// holdsOpenQuestionTx is the one test of whether a reader holds a domain's
// open question, shared by the list, the membership check and both answers.
func (s *Store) holdsOpenQuestionTx(ctx context.Context, tx pgx.Tx, domain string, reader ids.UUID) (bool, error) {
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, bind)
	if err != nil {
		return false, err
	}
	domainArg, readerArg := fmt.Sprintf("$%d", bind(domain)), fmt.Sprintf("$%d::uuid", bind(reader))
	open, err := openQuestionForReaderSQL(ctx, "d.domain", readerArg, identities, s.clock(), bind)
	if err != nil {
		return false, err
	}
	var holds bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM company_domain_disposition d
 WHERE d.domain = `+domainArg+` AND `+open+`
 AND NOT EXISTS (SELECT 1 FROM capture_exclusion e WHERE e.kind = 'domain' AND e.value = d.domain
 AND (e.scope = 'workspace' OR e.user_id = `+readerArg+`)))`, args...).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("contacts: reading who holds the question about %s: %w", domain, err)
	}
	return holds, nil
}

// Ownerless contacts leave the question unassigned instead of referencing a zero seat.
func domainQuestionOwner(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}
