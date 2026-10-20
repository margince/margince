// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type MergeCompanyFact struct {
	Source crmcontracts.CompanyFact
	Target *crmcontracts.CompanyFact
}

type MergeCompanyProfile struct {
	Source crmcontracts.CompanyProfileField
	Target *crmcontracts.CompanyProfileField
}

func previewCompanyMergeEvidence(ctx context.Context, tx pgx.Tx, source, target ids.UUID, out *MergeSurvivorship) error {
	from, to := ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target)
	sourceFacts, err := readCompanyFactsTx(ctx, tx, from)
	if err != nil {
		return err
	}
	targetFacts, err := readCompanyFactsTx(ctx, tx, to)
	if err != nil {
		return err
	}
	if len(sourceFacts) > 500 {
		return apperrors.ErrInvalidArgument
	}
	type factKey struct {
		category crmcontracts.CompanyFactCategory
		field    crmcontracts.CompanyFactField
		value    string
	}
	kept := map[factKey]crmcontracts.CompanyFact{}
	for _, fact := range targetFacts {
		kept[factKey{fact.Category, fact.Field, fact.ValueKey}] = fact
	}
	for _, fact := range sourceFacts {
		if err := previewFieldsVisible(ctx, "company", string(fact.Field)); err != nil {
			return err
		}
		carried := MergeCompanyFact{Source: fact}
		if survivor, found := kept[factKey{fact.Category, fact.Field, fact.ValueKey}]; found {
			carried.Target = &survivor
		}
		out.CompanyFacts = append(out.CompanyFacts, carried)
	}
	sourceProfile, err := readCompanyProfileFieldsTx(ctx, tx, from)
	if err != nil {
		return err
	}
	targetProfile, err := readCompanyProfileFieldsTx(ctx, tx, to)
	if err != nil {
		return err
	}
	if len(sourceProfile) > 500 {
		return apperrors.ErrInvalidArgument
	}
	profiles := map[crmcontracts.CompanyProfileFieldField]crmcontracts.CompanyProfileField{}
	for _, field := range targetProfile {
		profiles[field.Field] = field
	}
	for _, field := range sourceProfile {
		if err := previewFieldsVisible(ctx, "company", string(field.Field)); err != nil {
			return err
		}
		carried := MergeCompanyProfile{Source: field}
		if survivor, found := profiles[field.Field]; found {
			carried.Target = &survivor
		}
		out.CompanyProfiles = append(out.CompanyProfiles, carried)
	}
	if err := previewFieldsVisible(ctx, "company", "relationship_types"); err != nil {
		return err
	}
	types, err := readLiveRelationshipTypes(ctx, tx, from)
	if err != nil {
		return err
	}
	out.CompanyRelationshipTypes = types
	out.CompanySingletons, err = previewCompanySingletons(ctx, tx, source, target)
	return err
}
