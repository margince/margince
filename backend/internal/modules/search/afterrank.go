// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// decorate adds what a hit carries beyond its rank: tag reach, an email's
// summary, the partner mark and the company logo.
func (s *Store) decorate(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	if err := s.countTagReach(ctx, tx, hits); err != nil {
		return err
	}
	if err := s.attachEmailSummaries(ctx, tx, hits); err != nil {
		return err
	}
	if err := s.markPartners(ctx, tx, hits); err != nil {
		return err
	}
	return s.attachCompanyLogos(ctx, tx, hits)
}

// noteCaptureWithheld reports, on the page's own snapshot, that capture privacy
// kept a row of a searched type from the caller. The lists report it the same way.
func noteCaptureWithheld(ctx context.Context, tx pgx.Tx, types []string) error {
	for _, branch := range searchBranches {
		if !slices.Contains(types, branch.entity) {
			continue
		}
		if err := auth.NoteCaptureWithheld(ctx, tx, branch.table); err != nil {
			return err
		}
	}
	return nil
}
