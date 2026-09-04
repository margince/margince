// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SiteReadLogoKey reports where the mark an onboarding read resolved is stored,
// so the review can show the company it is about before a confirmation binds
// the mark to a record. ErrNotFound for a read that resolved none as much as
// for one that does not exist: the client draws the monogram for both, and
// telling them apart would say which dossiers exist.
//
// The same gate GetCompanySiteRead holds: the reader of a dossier is the one
// who may read the record it will become, or create it on the cold start.
func (s *Store) SiteReadLogoKey(ctx context.Context, readID ids.UUID) (string, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		if createErr := auth.Require(ctx, "organization", principal.ActionCreate); createErr != nil {
			return "", createErr
		}
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT logo_object_key FROM site_read WHERE id = $1 AND target_kind = 'onboarding'`, readID,
		).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("site read %s logo: %w", readID, err)
	}
	if key == nil || *key == "" {
		return "", apperrors.ErrNotFound
	}
	return *key, nil
}
