// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company and deal arms of the ingress port.
//
// Neither kind has a second write path. Each call goes through the product's
// own store. A unit's write gets what a user's edit gets: the rights check,
// row scope, audit row and outbox event.
//
// An update only fills fields that are empty. The core cannot tell a value a
// user typed from one a unit wrote earlier, so it keeps what is there.
// Stage moves are the one explicit change a unit may make. A switch for
// overwriting belongs in fillCompany and moveDeal, the only places that choose.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// crmLanding holds the stores one company or deal record is landed with.
type crmLanding struct {
	contacts   *contacts.Store
	deals      *deals.Store
	activities *activities.Store
	// system is the provenance this unit's records carry (extSourceSystem).
	system string
}

// ingestOtherKind lands a company or deal under the member's live authority.
func (r *callRuntime) ingestOtherKind(ctx context.Context, on extension.UserID, rec extension.Record) (extension.Result, error) {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return extension.Result{}, err
	}
	runCtx, err := r.ingressAuthority(ctx, on)
	if err != nil {
		return extension.Result{}, err
	}
	db := InstallationDB(r.deps.pool)
	land := crmLanding{
		contacts:   contacts.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()),
		activities: activities.NewStore(db),
		system:     r.sourceSystem(rec.System),
	}
	var ref extension.Ref
	if rec.Kind() == extension.KindCompany {
		ref, err = land.company(runCtx, rec.Key, *rec.Company)
	} else {
		ref, err = land.deal(runCtx, rec.Key, *rec.Deal)
	}
	if err != nil {
		return extension.Result{}, r.ingressRefusal(runCtx, err)
	}
	return extension.Result{Ref: ref, Disposition: extension.DispositionAccepted}, nil
}

// upsertByOutsideID finds the holder of an outside id, or creates one.
//
// A create that hits a conflict looks once more. Either it lost a race to a
// second call with the same id, and that call's record is now the holder to
// update. Or it met a record the member may not see, and that stays a conflict.
// The answer is the same fixed conflict whoever asks, because find reads only
// what the member may see.
func upsertByOutsideID(
	find func() (ids.UUID, bool, error), create func() (ids.UUID, error), update func(ids.UUID) error,
) (ids.UUID, error) {
	id, found, err := find()
	if err != nil {
		return ids.Nil, err
	}
	if !found {
		id, err = create()
		if !errors.Is(err, apperrors.ErrConflict) {
			return id, err
		}
		var again bool
		if id, again, err = find(); err != nil || !again {
			return ids.Nil, errors.Join(apperrors.ErrConflict, err)
		}
	}
	return id, update(id)
}

func (l crmLanding) company(ctx context.Context, key string, f extension.CompanyFields) (extension.Ref, error) {
	one := 1
	var adopt *contacts.OutsideID
	find := func() (ids.UUID, bool, error) {
		rows, _, err := l.contacts.ListCompanies(ctx, contacts.ListCompaniesInput{SourceSystem: &l.system, SourceID: &key, Limit: &one})
		switch {
		case err != nil:
			return ids.Nil, false, err
		case len(rows) > 0:
			return ids.UUID(rows[0].Id), true, nil
		case f.AdoptOn != extension.AdoptKeyDomain:
			return ids.Nil, false, nil
		}
		held, err := l.companyByDomain(ctx, f.Domain)
		if err != nil || held.IsZero() {
			return ids.Nil, false, err
		}
		adopt = &contacts.OutsideID{System: l.system, ID: key}
		return held, true, nil
	}
	create := func() (ids.UUID, error) {
		made, err := l.contacts.CreateCompany(ctx, contacts.CreateCompanyInput{
			DisplayName: f.DisplayName, LegalName: nonEmpty(f.LegalName), Description: nonEmpty(f.Description),
			Industry: nonEmpty(f.Industry), Source: l.system, SourceSystem: &l.system, SourceID: &key,
			Domains: domainsOf(f.Domain),
		})
		return ids.UUID(made.Id), err
	}
	update := func(id ids.UUID) error {
		in, err := l.fillCompany(ctx, id, f)
		if err != nil {
			return err
		}
		in.AdoptSource = adopt
		_, err = l.contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), in)
		return err
	}
	id, err := upsertByOutsideID(find, create, update)
	if err == nil {
		err = l.relink(ctx, f.RelinkActivities, string(recordTypeCompany), id)
	}
	return extension.Ref{Type: string(recordTypeCompany), ID: id.String()}, err
}

// companyByDomain answers the one company that lists the domain, or none. A
// company the member cannot see is none, and creating beside it then meets the
// domain's own uniqueness rule.
func (l crmLanding) companyByDomain(ctx context.Context, domain string) (ids.UUID, error) {
	two := 2
	rows, _, err := l.contacts.ListCompanies(ctx, contacts.ListCompaniesInput{Domain: &domain, Limit: &two})
	switch {
	case err != nil || len(rows) == 0:
		return ids.Nil, err
	case len(rows) > 1:
		return ids.Nil, fmt.Errorf("several companies list this domain: %w", apperrors.ErrConflict)
	}
	return ids.UUID(rows[0].Id), nil
}

// fillCompany is the edit that sets only what is empty.
func (l crmLanding) fillCompany(ctx context.Context, id ids.UUID, f extension.CompanyFields) (contacts.UpdateCompanyInput, error) {
	current, err := l.contacts.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.LiveOnly)
	if err != nil {
		return contacts.UpdateCompanyInput{}, err
	}
	return contacts.UpdateCompanyInput{
		LegalName:   fillIfEmpty(current.LegalName, f.LegalName),
		Industry:    fillIfEmpty(current.Industry, f.Industry),
		Description: fillIfEmpty(current.Description, f.Description),
	}, nil
}

func (l crmLanding) deal(ctx context.Context, key string, f extension.DealFields) (extension.Ref, error) {
	pipeline, err := ids.Parse(f.PipelineID)
	if err != nil {
		return extension.Ref{}, fmt.Errorf("%w: the pipeline id is not a canonical UUID", extension.ErrInvalid)
	}
	stage, err := ids.Parse(f.StageID)
	if err != nil {
		return extension.Ref{}, fmt.Errorf("%w: the stage id is not a canonical UUID", extension.ErrInvalid)
	}
	one := 1
	find := func() (ids.UUID, bool, error) {
		rows, _, err := l.deals.ListDeals(ctx, deals.ListDealsInput{SourceSystem: &l.system, SourceID: &key, Limit: &one})
		if err != nil || len(rows) == 0 {
			return ids.Nil, false, err
		}
		return ids.UUID(rows[0].Id), true, nil
	}
	create := func() (ids.UUID, error) {
		made, err := l.deals.CreateDeal(ctx, deals.CreateDealInput{
			Name: f.Name, AmountMinor: f.AmountMinor, Currency: nonEmpty(f.Currency),
			PipelineID: ids.From[ids.PipelineKind](pipeline), StageID: ids.From[ids.StageKind](stage),
			ReferencesFromRequest: true, Source: l.system, SourceSystem: &l.system, SourceID: &key,
		})
		return ids.UUID(made.Id), err
	}
	update := func(id ids.UUID) error { return l.moveDeal(ctx, id, stage, f) }
	id, err := upsertByOutsideID(find, create, update)
	if err == nil {
		err = l.relink(ctx, f.RelinkActivities, string(recordTypeDeal), id)
	}
	return extension.Ref{Type: string(recordTypeDeal), ID: id.String()}, err
}

// moveDeal fills the empty value fields and moves the stage when it differs.
func (l crmLanding) moveDeal(ctx context.Context, id, stage ids.UUID, f extension.DealFields) error {
	dealID := ids.From[ids.DealKind](id)
	current, err := l.deals.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	in := deals.UpdateDealInput{}
	if current.AmountMinor == nil {
		in.AmountMinor = f.AmountMinor
	}
	if current.Currency == nil {
		in.Currency = nonEmpty(f.Currency)
	}
	if _, err := l.deals.UpdateDeal(ctx, dealID, in); err != nil {
		return err
	}
	if current.StageId != nil && ids.UUID(*current.StageId) == stage {
		return nil
	}
	// A win a unit reports has no contract in this system; it is an imported win.
	imported := string(crmcontracts.DealWonWithoutContractReasonImported)
	_, err = l.deals.AdvanceDeal(ctx, dealID, deals.AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](stage), ReferencesFromRequest: true,
		LostReason: nonEmpty(f.LostReason), WonWithoutContractReason: &imported,
	})
	return err
}

// relink moves each named activity onto the record. A key that matches nothing
// the member may read is skipped without a word, so the list cannot be used to
// probe for activities.
func (l crmLanding) relink(ctx context.Context, keys []string, entityType string, target ids.UUID) error {
	for _, key := range slices.Compact(slices.Clone(keys)) {
		activity, err := l.activities.ActivityIDBySource(ctx, l.system, key)
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := l.activities.RelinkActivity(ctx, activity, activities.RelinkActivityInput{EntityType: entityType, EntityID: target}); err != nil {
			return err
		}
	}
	return nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// fillIfEmpty answers the unit's value only where the record holds none.
func fillIfEmpty(current *string, offered string) *string {
	if offered == "" || (current != nil && *current != "") {
		return nil
	}
	return &offered
}

func domainsOf(domain string) []contacts.CompanyDomainInput {
	if domain == "" {
		return nil
	}
	return []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}}
}
