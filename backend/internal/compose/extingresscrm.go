// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company and deal arms of the ingress port.
//
// Neither kind has a second write path. Each call goes through the product's
// own store. A unit's write gets what a user's edit gets: the rights check,
// row scope, audit row and outbox event.
//
// An update writes the fields the unit offers, except the ones a user last
// edited: humanprecedence.go answers that from the audit trail. Whether a unit
// may overwrite those is the installation's per-unit setting, read before the
// check. With it off, the withheld fields come back in the Result. Stage moves
// are the one change a unit makes without that check.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
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
	owner  fieldOwnership
	// overwrite lets the unit's offer replace a field a user last edited.
	overwrite bool
}

// ingestOtherKind lands a company, deal or signal under the member's live authority.
func (r *callRuntime) ingestOtherKind(ctx context.Context, on extension.UserID, rec extension.Record, declared extension.IngressSource) (extension.Result, error) {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return extension.Result{}, err
	}
	runCtx, err := r.ingressAuthority(ctx, on)
	if err != nil {
		return extension.Result{}, err
	}
	if rec.Kind() == extension.KindSignal {
		return r.landSignal(runCtx, rec, declared)
	}
	db := InstallationDB(r.deps.pool)
	overwrite, err := r.overwritesEditedFields(ctx)
	if err != nil {
		return extension.Result{}, err
	}
	land := crmLanding{
		contacts:   contacts.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()).WithFieldCatalog(customfields.NewService(r.deps.pool, nil)),
		activities: activities.NewStore(db),
		system:     r.sourceSystem(rec.System),
		owner:      fieldOwnership{pool: r.deps.pool},
		overwrite:  overwrite,
	}
	var ref extension.Ref
	var skipped []string
	if rec.Kind() == extension.KindCompany {
		ref, skipped, err = land.company(runCtx, rec.Key, *rec.Company)
	} else {
		ref, skipped, err = land.deal(runCtx, rec.Key, *rec.Deal)
	}
	if err != nil {
		return extension.Result{}, r.ingressRefusal(runCtx, err)
	}
	return extension.Result{Ref: ref, Disposition: extension.DispositionAccepted, Withheld: skipped}, nil
}

// overwritesEditedFields reads this unit's setting. The read is ungated: the
// feed's service member holds no settings grant, and the posture must bind it.
func (r *callRuntime) overwritesEditedFields(ctx context.Context) (bool, error) {
	var perUnit map[string]bool
	err := database.WithWorkspaceTx(ctx, r.deps.pool, func(tx pgx.Tx) (err error) {
		perUnit, err = settings.ApplyTx(ctx, tx, identity.ExtensionOverwritesEditedFields)
		return err
	})
	return perUnit[r.unit], err
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

func (l crmLanding) company(ctx context.Context, key string, f extension.CompanyFields) (extension.Ref, []string, error) {
	one := 1
	var skipped []string
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
		in, left, err := l.fillCompany(ctx, id, f)
		if err != nil {
			return err
		}
		skipped = left
		in.AdoptSource = adopt
		_, err = l.contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), in)
		return err
	}
	id, err := upsertByOutsideID(find, create, update)
	if err == nil {
		err = l.contacts.RecordOfferedValues(ctx, ids.From[ids.CompanyKind](id), l.system, offeredCompanyFields(f))
	}
	if err == nil {
		err = l.relink(ctx, f.RelinkActivities, string(recordTypeCompany), id)
	}
	return extension.Ref{Type: string(recordTypeCompany), ID: id.String()}, skipped, err
}

// offeredCompanyFields is what the unit offers for the fields a user may
// also type, kept whether or not the record takes it.
func offeredCompanyFields(f extension.CompanyFields) map[string]string {
	offered := map[string]string{}
	for key, v := range map[string]string{fieldLegalName: f.LegalName, fieldIndustry: f.Industry, descriptionField: f.Description} {
		if v != "" {
			offered[key] = v
		}
	}
	return offered
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

// fillCompany is the edit of the fields the unit offers that differ from the
// record, less the ones a user owns. Those come back as the withheld names.
func (l crmLanding) fillCompany(ctx context.Context, id ids.UUID, f extension.CompanyFields) (contacts.UpdateCompanyInput, []string, error) {
	current, err := l.contacts.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.LiveOnly)
	if err != nil {
		return contacts.UpdateCompanyInput{}, nil, err
	}
	offer := map[string]*string{
		fieldLegalName:   changedText(current.LegalName, f.LegalName),
		fieldIndustry:    changedText(industryText(current.Industry), f.Industry),
		descriptionField: changedText(current.Description, f.Description),
	}
	patch := map[string]any{}
	for key, v := range offer {
		if v != nil {
			patch[key] = *v
		}
	}
	skipped, err := l.skippedFields(ctx, string(recordTypeCompany), id, patch)
	if err != nil {
		return contacts.UpdateCompanyInput{}, nil, err
	}
	return contacts.UpdateCompanyInput{
		LegalName:   writable(offer, skipped, fieldLegalName),
		Industry:    writable(offer, skipped, fieldIndustry),
		Description: writable(offer, skipped, descriptionField),
	}, skipped, nil
}

// skippedFields names the fields of patch that a user last edited, unless
// the unit may overwrite them. Nothing is read when there is nothing to check.
func (l crmLanding) skippedFields(ctx context.Context, entityType string, id ids.UUID, patch map[string]any) ([]string, error) {
	if l.overwrite || len(patch) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("compose: encoding the offered fields: %w", err)
	}
	return l.owner.HumanOwnedConflicts(ctx, entityType, id, raw)
}

// writable answers the offered value of key unless it was skipped.
func writable(offer map[string]*string, skipped []string, key string) *string {
	if slices.Contains(skipped, key) {
		return nil
	}
	return offer[key]
}

// industryText is the stored industry code as the text a unit offers, so the
// two compare as written.
func industryText(code *crmcontracts.CompanyIndustry) *string {
	if code == nil {
		return nil
	}
	text := string(*code)
	return &text
}

// changedText answers the offered value only where it differs from the record.
func changedText(current *string, offered string) *string {
	if offered == "" || (current != nil && *current == offered) {
		return nil
	}
	return &offered
}

func (l crmLanding) deal(ctx context.Context, key string, f extension.DealFields) (extension.Ref, []string, error) {
	pipeline, err := ids.Parse(f.PipelineID)
	if err != nil {
		return extension.Ref{}, nil, fmt.Errorf("%w: the pipeline id is not a canonical UUID", extension.ErrInvalid)
	}
	stage, err := ids.Parse(f.StageID)
	if err != nil {
		return extension.Ref{}, nil, fmt.Errorf("%w: the stage id is not a canonical UUID", extension.ErrInvalid)
	}
	var company *ids.CompanyID
	if f.CompanyID != "" {
		parsed, err := ids.Parse(f.CompanyID)
		if err != nil {
			return extension.Ref{}, nil, fmt.Errorf("%w: the company id is not a canonical UUID", extension.ErrInvalid)
		}
		company = new(ids.From[ids.CompanyKind](parsed))
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
			Name: f.Name, AmountMinor: f.AmountMinor, Currency: nonEmpty(f.Currency), CustomFields: f.CustomFields, CompanyID: company,
			PipelineID: ids.From[ids.PipelineKind](pipeline), StageID: ids.From[ids.StageKind](stage),
			ReferencesFromRequest: true, Source: l.system, SourceSystem: &l.system, SourceID: &key,
		})
		return ids.UUID(made.Id), err
	}
	var skipped []string
	update := func(id ids.UUID) (err error) {
		skipped, err = l.moveDeal(ctx, id, stage, f)
		return err
	}
	id, err := upsertByOutsideID(find, create, update)
	if err == nil {
		err = l.relink(ctx, f.RelinkActivities, string(recordTypeDeal), id)
	}
	return extension.Ref{Type: string(recordTypeDeal), ID: id.String()}, skipped, err
}

// moveDeal writes the value fields the unit offers that differ from the deal,
// less the ones a user owns. It moves the stage when the stage differs.
func (l crmLanding) moveDeal(ctx context.Context, id, stage ids.UUID, f extension.DealFields) ([]string, error) {
	dealID := ids.From[ids.DealKind](id)
	current, err := l.deals.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		return nil, err
	}
	amount := f.AmountMinor
	if amount != nil && current.AmountMinor != nil && *amount == *current.AmountMinor {
		amount = nil
	}
	currency := changedText(current.Currency, f.Currency)
	patch := map[string]any{}
	if amount != nil {
		patch[acceptFieldAmountMinor] = *amount
	}
	if currency != nil {
		patch[dealTargetCurrency] = *currency
	}
	// An amount and its currency are one figure. Writing the half a user did
	// not edit would price their number in another currency. So the half the
	// unit did not offer is asked about too, as a null no stored value equals.
	// A user's edit to either half withholds both.
	if len(patch) > 0 {
		for _, key := range []string{acceptFieldAmountMinor, dealTargetCurrency} {
			if _, offered := patch[key]; !offered {
				patch[key] = nil
			}
		}
	}
	skipped, err := l.skippedFields(ctx, string(recordTypeDeal), id, patch)
	if err != nil {
		return nil, err
	}
	in := deals.UpdateDealInput{}
	if len(skipped) > 0 {
		skipped = slices.Sorted(maps.Keys(patch))
	} else {
		in.AmountMinor, in.Currency = amount, currency
	}
	custom, heldBack, err := l.customToWrite(ctx, id, current, f.CustomFields)
	if err != nil {
		return nil, err
	}
	in.CustomFields = custom
	skipped = append(skipped, heldBack...)
	if _, err := l.deals.UpdateDeal(ctx, dealID, in); err != nil {
		return nil, err
	}
	return skipped, l.advanceDeal(ctx, dealID, current, stage, f)
}

// customToWrite answers the offered custom fields that differ from the deal and
// that no user last edited, and names the ones a user owns. A field has no
// pairing like amount and currency, so each is held back on its own.
func (l crmLanding) customToWrite(ctx context.Context, id ids.UUID, current crmcontracts.Deal, offered map[string]any) (map[string]any, []string, error) {
	if len(offered) == 0 {
		return nil, nil, nil
	}
	// The stored value reads back as decoded JSON. The offer is put through JSON
	// too, or a []string would never equal the []any it was saved as.
	raw, err := json.Marshal(offered)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: encoding the offered custom fields: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the offered custom fields back: %w", err)
	}
	changed := map[string]any{}
	for key, value := range normalized {
		if !reflect.DeepEqual(current.AdditionalProperties[key], value) {
			changed[key] = value
		}
	}
	held, err := l.skippedFields(ctx, string(recordTypeDeal), id, changed)
	if err != nil {
		return nil, nil, err
	}
	for _, key := range held {
		delete(changed, key)
	}
	slices.Sort(held)
	return changed, held, nil
}

// advanceDeal moves the stage when it differs. A stage move is the one change
// a unit makes whoever last edited the deal.
func (l crmLanding) advanceDeal(ctx context.Context, dealID ids.DealID, current crmcontracts.Deal, stage ids.UUID, f extension.DealFields) error {
	if current.StageId != nil && ids.UUID(*current.StageId) == stage {
		return nil
	}
	// A win a unit reports has no contract in this system; it is an imported win.
	imported := string(crmcontracts.DealWonWithoutContractReasonImported)
	_, err := l.deals.AdvanceDeal(ctx, dealID, deals.AdvanceDealInput{
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

func domainsOf(domain string) []contacts.CompanyDomainInput {
	if domain == "" {
		return nil
	}
	return []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}}
}
