// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type disclosureAdapter struct {
	store *consent.Store
}

// DisclosuresFor keeps pack resolution in consent and rendering in activities.
func (a disclosureAdapter) DisclosuresFor(
	ctx context.Context, category string, recipients []string, countries []commsauthz.CountryOverride, entity ids.UUID, language string,
) ([]activities.DisclosureLine, error) {
	owed, err := a.store.Disclosures(ctx, consent.DisclosureQuery{Category: commsauthz.Category(category), Recipients: recipients, Countries: countries, Entity: entity, Language: language})
	if err != nil {
		return nil, err
	}
	out := make([]activities.DisclosureLine, 0, len(owed))
	for _, line := range owed {
		out = append(out, activities.DisclosureLine{Kind: line.Kind, Text: line.Text, MissingLanguage: line.MissingLanguage, MissingMeans: line.MissingMeans})
	}
	return out, nil
}

func newDisclosureAdapter(db *database.DB) disclosureAdapter {
	return disclosureAdapter{store: consent.NewStore(db).
		WithInstallationCountry(consent.InstallationCountryFunc(identity.CountryOf)).
		WithMailLanguage(consent.MailLanguageFunc(identity.LanguageOf))}
}
