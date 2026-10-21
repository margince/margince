// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readyTemplateCreation(in CreateOfferTemplateInput) CreateOfferTemplateInput {
	if in.Locale == "" {
		in.Locale = defaultOfferTemplateLocale
	}
	in.Layout = maps.Clone(in.Layout)
	if in.Layout == nil {
		in.Layout = map[string]any{}
	}
	return in
}

func (s *Store) admitTemplateCreation(ctx context.Context, tx pgx.Tx, in CreateOfferTemplateInput) error {
	var noExclusion ids.OfferTemplateID
	if err := s.checkTemplateNameConflict(ctx, tx, noExclusion, in.Name); err != nil {
		return err
	}
	if in.IsDefault {
		return s.checkTemplateDefaultConflict(ctx, tx, noExclusion, in.Locale)
	}
	return nil
}

// PreviewTemplateCreation shares defaults and conflict checks with the template writer.
func (s *Store) PreviewTemplateCreation(ctx context.Context, in CreateOfferTemplateInput) (CreateOfferTemplateInput, error) {
	if err := auth.Require(ctx, "offer_template", principal.ActionCreate); err != nil {
		return CreateOfferTemplateInput{}, err
	}
	in = readyTemplateCreation(in)
	if err := s.Tx(ctx, func(tx pgx.Tx) error { return s.admitTemplateCreation(ctx, tx, in) }); err != nil {
		return CreateOfferTemplateInput{}, err
	}
	return in, nil
}
