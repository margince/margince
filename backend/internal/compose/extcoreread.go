// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of the read verbs on extension.Core.
//
// Each verb runs the product's own transaction-taking read. Grant, row scope
// and field masks are the store's, not a second copy. This file adds what only
// the door knows. A tick has no caller. A unit has a rate. A bulk read is an
// export. The catalog is read on the unit's own transaction.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

// extensionReadsPerMinute is how many core reads one unit may make in a minute,
// across all its invocations. With pages of at most extension.MaxReadPage
// records, that bounds a unit to tens of thousands of records a minute.
const extensionReadsPerMinute = 600

// extensionReads meters reads per unit. FailOpen: a read costs only traffic, and
// refusing every unit's read because the shared counter is down would stop work
// no attacker gains from.
var extensionReads = ratelimit.New("extension-core-read", ratelimit.FailOpen, extensionReadsPerMinute, time.Minute)

// The keys of an export row's detail, and the filter names a list records in it.
const (
	filterKind       = "kind"
	filterSearch     = "search"
	filterStage      = "stage_id"
	filterStatus     = "status"
	filterSource     = "source"
	filterOwner      = "owner_id"
	filterCompany    = "company_id"
	filterResolution = "resolution_state"
)

// txCatalog reads the custom-field catalog on the transaction a unit holds, where
// the store's own reader would take a second connection.
type txCatalog struct{ tx pgx.Tx }

func (c txCatalog) ActiveColumns(ctx context.Context, object string) ([]fieldcatalog.Column, error) {
	return customfields.ActiveColumnsIn(ctx, c.tx, object)
}

// beginRead is what every read verb does first: refuse a tick, bind the
// invocation's authority and spend one of the unit's reads.
func (c extensionCore) beginRead(ctx context.Context) (context.Context, error) {
	if err := c.refuseUnattended(); err != nil {
		return nil, err
	}
	ctx, err := c.invocationCtx(ctx)
	if err != nil {
		return nil, err
	}
	if !extensionReads.Allow(c.unit) {
		return nil, extension.ErrRateLimited
	}
	return ctx, nil
}

func (c extensionCore) catalog() txCatalog { return txCatalog{tx: c.tx} }

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.DealRepo, never a core type.
func (c extensionCore) Deals() extension.DealRepo { return extensionDeals{core: c} }

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.CompanyRepo, never a core type.
func (c extensionCore) Companies() extension.CompanyRepo { return extensionCompanies{core: c} }

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.SignalRepo, never a core type.
func (c extensionCore) Signals() extension.SignalRepo { return extensionSignals{core: c} }

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.CustomFieldRepo, never a core type.
func (c extensionCore) CustomFields() extension.CustomFieldRepo {
	return extensionCustomFields{core: c}
}

// readLimit turns the published limit into the store's: zero is the default, and
// a larger one is cut to the page ceiling.
func readLimit(limit int) (*int, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%w: limit cannot be negative", extension.ErrInvalid)
	}
	return pageLimit(min(limit, extension.MaxReadPage)), nil
}

// pageLimit is nil for zero, which the stores read as "use the default".
func pageLimit(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// idFilter parses an id filter, where empty means no filter.
func idFilter[K ids.EntityKind](s string) (*ids.ID[K], error) {
	var id *ids.ID[K]
	if s == "" {
		return id, nil
	}
	parsed, err := ids.ParseAs[K](s)
	if err != nil {
		return id, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, s)
	}
	return &parsed, nil
}

// bothOrNeither refuses a lookup that names half of a pair.
func bothOrNeither(a, b, what string) error {
	if (a == "") != (b == "") {
		return fmt.Errorf("%w: name both halves of %s, or neither", extension.ErrInvalid, what)
	}
	return nil
}

// usedFilter is one filter of a query and the value it was given.
type usedFilter struct{ name, value string }

// filterNames lists the filters a query used, for the audit row.
func filterNames(custom map[string]string, used ...usedFilter) []string {
	var names []string
	for _, f := range used {
		if f.value != "" {
			names = append(names, f.name)
		}
	}
	if len(custom) > 0 {
		names = append(names, "custom_fields")
	}
	return names
}

// publish carries one store page onto the published shape through the shared
// JSON form, as every other verb here does.
func publish[T, S any](items []S, page storekit.Page) (extension.Page[T], error) {
	out, err := transcode[[]T](items)
	if err != nil {
		return extension.Page[T]{}, fmt.Errorf("compose: a record the store read does not fit the published shape: %w", err)
	}
	if out == nil {
		out = []T{}
	}
	return extension.Page[T]{Items: out, NextCursor: page.NextCursor}, nil
}

func publishOne[T, S any](record S) (T, error) {
	out, err := transcode[T](record)
	if err != nil {
		return out, fmt.Errorf("compose: a record the store read does not fit the published shape: %w", err)
	}
	return out, nil
}

type extensionDeals struct{ core extensionCore }

func (d extensionDeals) store() *deals.Store {
	return deals.NewStore(InstallationDB(d.core.deps.pool), DealsInstallation())
}

func (d extensionDeals) Get(ctx context.Context, id string) (crm.Deal, error) {
	ctx, err := d.core.beginRead(ctx)
	if err != nil {
		return crm.Deal{}, err
	}
	dealID, err := ids.ParseAs[ids.DealKind](id)
	if err != nil {
		return crm.Deal{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	cols, err := deals.DealColumnsFrom(ctx, d.core.catalog())
	if err != nil {
		return crm.Deal{}, portRefusal(err)
	}
	got, err := d.store().GetDealTx(ctx, d.core.tx, dealID, storekit.LiveOnly, cols)
	if err != nil {
		return crm.Deal{}, portRefusal(err)
	}
	return publishOne[crm.Deal](got)
}

func (d extensionDeals) List(ctx context.Context, q extension.DealQuery) (extension.Page[crm.Deal], error) {
	ctx, err := d.core.beginRead(ctx)
	if err != nil {
		return extension.Page[crm.Deal]{}, err
	}
	in, err := dealInput(q)
	if err != nil {
		return extension.Page[crm.Deal]{}, err
	}
	cols, err := deals.DealColumnsFrom(ctx, d.core.catalog())
	if err != nil {
		return extension.Page[crm.Deal]{}, portRefusal(err)
	}
	rows, page, err := d.store().ListDealsTx(ctx, d.core.tx, in, cols)
	if err != nil {
		return extension.Page[crm.Deal]{}, portRefusal(err)
	}
	names := filterNames(q.CustomFields,
		usedFilter{filterSearch, q.Search}, usedFilter{filterStage, q.StageID}, usedFilter{filterOwner, q.OwnerID},
		usedFilter{filterCompany, q.CompanyID}, usedFilter{filterStatus, q.Status}, usedFilter{filterSource, q.SourceID})
	if err := d.core.auditBulkRead(string(recordTypeDeal), len(rows), names); err != nil {
		return extension.Page[crm.Deal]{}, portRefusal(err)
	}
	return publish[crm.Deal](rows, page)
}

func dealInput(q extension.DealQuery) (deals.ListDealsInput, error) {
	limit, err := readLimit(q.Limit)
	if err != nil {
		return deals.ListDealsInput{}, err
	}
	if err := bothOrNeither(q.SourceSystem, q.SourceID, "the outside id"); err != nil {
		return deals.ListDealsInput{}, err
	}
	stage, err := idFilter[ids.StageKind](q.StageID)
	if err != nil {
		return deals.ListDealsInput{}, err
	}
	owner, err := idFilter[ids.UserKind](q.OwnerID)
	if err != nil {
		return deals.ListDealsInput{}, err
	}
	company, err := idFilter[ids.CompanyKind](q.CompanyID)
	if err != nil {
		return deals.ListDealsInput{}, err
	}
	return deals.ListDealsInput{
		Cursor: optional(q.Cursor), Limit: limit, Query: optional(q.Search),
		StageID: stage, OwnerID: owner, CompanyID: company, Status: optional(q.Status),
		SourceSystem: optional(q.SourceSystem), SourceID: optional(q.SourceID),
		CustomFilters: q.CustomFields, IncludeArchived: q.IncludeArchived,
	}, nil
}

type extensionCompanies struct{ core extensionCore }

func (c extensionCompanies) store() *contacts.Store {
	return contacts.NewStore(InstallationDB(c.core.deps.pool))
}

func (c extensionCompanies) Get(ctx context.Context, id string) (crm.Company, error) {
	ctx, err := c.core.beginRead(ctx)
	if err != nil {
		return crm.Company{}, err
	}
	companyID, err := ids.ParseAs[ids.CompanyKind](id)
	if err != nil {
		return crm.Company{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	cols, err := contacts.CompanyColumnsFrom(ctx, c.core.catalog())
	if err != nil {
		return crm.Company{}, portRefusal(err)
	}
	got, err := c.store().GetCompanyTx(ctx, c.core.tx, companyID, storekit.LiveOnly, cols)
	if err != nil {
		return crm.Company{}, portRefusal(err)
	}
	return publishOne[crm.Company](got)
}

func (c extensionCompanies) List(ctx context.Context, q extension.CompanyQuery) (extension.Page[crm.Company], error) {
	ctx, err := c.core.beginRead(ctx)
	if err != nil {
		return extension.Page[crm.Company]{}, err
	}
	limit, err := readLimit(q.Limit)
	if err != nil {
		return extension.Page[crm.Company]{}, err
	}
	if err := bothOrNeither(q.SourceSystem, q.SourceID, "the outside id"); err != nil {
		return extension.Page[crm.Company]{}, err
	}
	owner, err := idFilter[ids.UserKind](q.OwnerID)
	if err != nil {
		return extension.Page[crm.Company]{}, err
	}
	cols, err := contacts.CompanyColumnsFrom(ctx, c.core.catalog())
	if err != nil {
		return extension.Page[crm.Company]{}, portRefusal(err)
	}
	rows, page, err := c.store().ListCompaniesTx(ctx, c.core.tx, contacts.ListCompaniesInput{
		Cursor: optional(q.Cursor), Limit: limit, Query: optional(q.Search), OwnerID: owner,
		Domain: optional(q.Domain), Lifecycle: optional(q.Lifecycle),
		SourceSystem: optional(q.SourceSystem), SourceID: optional(q.SourceID),
		CustomFilters: q.CustomFields, IncludeArchived: q.IncludeArchived,
	}, cols)
	if err != nil {
		return extension.Page[crm.Company]{}, portRefusal(err)
	}
	names := filterNames(q.CustomFields,
		usedFilter{filterSearch, q.Search}, usedFilter{filterOwner, q.OwnerID}, usedFilter{"domain", q.Domain},
		usedFilter{"lifecycle", q.Lifecycle}, usedFilter{filterSource, q.SourceID})
	if err := c.core.auditBulkRead(string(recordTypeCompany), len(rows), names); err != nil {
		return extension.Page[crm.Company]{}, portRefusal(err)
	}
	return publish[crm.Company](rows, page)
}

type extensionSignals struct{ core extensionCore }

func (s extensionSignals) Get(ctx context.Context, id string) (crm.Signal, error) {
	ctx, err := s.core.beginRead(ctx)
	if err != nil {
		return crm.Signal{}, err
	}
	signalID, err := ids.ParseAs[ids.SignalKind](id)
	if err != nil {
		return crm.Signal{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	got, err := signals.ReadSignalTx(ctx, s.core.tx, signalID, storekit.LiveOnly)
	if err != nil {
		return crm.Signal{}, portRefusal(err)
	}
	return publishOne[crm.Signal](got)
}

func (s extensionSignals) List(ctx context.Context, q extension.SignalQuery) (extension.Page[crm.Signal], error) {
	ctx, err := s.core.beginRead(ctx)
	if err != nil {
		return extension.Page[crm.Signal]{}, err
	}
	limit, err := readLimit(q.Limit)
	if err != nil {
		return extension.Page[crm.Signal]{}, err
	}
	company, err := idFilter[ids.CompanyKind](q.CompanyID)
	if err != nil {
		return extension.Page[crm.Signal]{}, err
	}
	in := signals.ListSignalsInput{
		Cursor: optional(q.Cursor), Limit: limit, Kind: optional(q.Kind), Status: optional(q.Status),
		ResolutionState: optional(q.ResolutionState), IncludeArchived: q.IncludeArchived,
	}
	if company != nil {
		in.CompanyID = &company.UUID
	}
	rows, page, err := signals.ListSignalsTx(ctx, s.core.tx, in)
	if err != nil {
		return extension.Page[crm.Signal]{}, portRefusal(err)
	}
	names := filterNames(nil,
		usedFilter{filterKind, q.Kind}, usedFilter{filterStatus, q.Status},
		usedFilter{filterResolution, q.ResolutionState}, usedFilter{filterCompany, q.CompanyID})
	if err := s.core.auditBulkRead("signal", len(rows), names); err != nil {
		return extension.Page[crm.Signal]{}, portRefusal(err)
	}
	return publish[crm.Signal](rows, page)
}

func (a extensionActivities) Get(ctx context.Context, id string) (crm.Activity, error) {
	ctx, err := a.core.beginRead(ctx)
	if err != nil {
		return crm.Activity{}, err
	}
	activityID, err := ids.ParseAs[ids.ActivityKind](id)
	if err != nil {
		return crm.Activity{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	cols, err := activities.ActivityColumnsFrom(ctx, a.core.catalog())
	if err != nil {
		return crm.Activity{}, portRefusal(err)
	}
	got, err := a.store().GetActivityTx(ctx, a.core.tx, activityID, storekit.LiveOnly, cols)
	if err != nil {
		return crm.Activity{}, portRefusal(err)
	}
	return publishOne[crm.Activity](got)
}

func (a extensionActivities) List(ctx context.Context, q extension.ActivityQuery) (extension.Page[crm.Activity], error) {
	ctx, err := a.core.beginRead(ctx)
	if err != nil {
		return extension.Page[crm.Activity]{}, err
	}
	limit, err := readLimit(q.Limit)
	if err != nil {
		return extension.Page[crm.Activity]{}, err
	}
	if err := bothOrNeither(q.EntityType, q.EntityID, "the record link"); err != nil {
		return extension.Page[crm.Activity]{}, err
	}
	in := activities.ListActivitiesInput{
		Cursor: optional(q.Cursor), Limit: limit, Kind: optional(q.Kind), Query: optional(q.Search),
		EntityType: optional(q.EntityType), CustomFilters: q.CustomFields, IncludeArchived: q.IncludeArchived,
	}
	if q.EntityID != "" {
		entity, err := ids.Parse(q.EntityID)
		if err != nil {
			return extension.Page[crm.Activity]{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, q.EntityID)
		}
		in.EntityID = &entity
	}
	if in.Columns, err = activities.ActivityColumnsFrom(ctx, a.core.catalog()); err != nil {
		return extension.Page[crm.Activity]{}, portRefusal(err)
	}
	rows, page, err := a.store().ListActivitiesIn(ctx, a.core.tx, in)
	if err != nil {
		return extension.Page[crm.Activity]{}, portRefusal(err)
	}
	names := filterNames(q.CustomFields,
		usedFilter{filterKind, q.Kind}, usedFilter{filterSearch, q.Search}, usedFilter{"entity", q.EntityID})
	if err := a.core.auditBulkRead(string(recordTypeActivity), len(rows), names); err != nil {
		return extension.Page[crm.Activity]{}, portRefusal(err)
	}
	return publish[crm.Activity](rows, page)
}

func (a extensionActivities) store() *activities.Store {
	return activities.NewStore(InstallationDB(a.core.deps.pool))
}

type extensionCustomFields struct{ core extensionCore }

// customFieldObjects are the record types a unit may list fields of: the ones
// whose records it can read.
var customFieldObjects = []string{string(recordTypeDeal), string(recordTypeCompany), string(recordTypeActivity)}

func (f extensionCustomFields) List(ctx context.Context, object string) ([]extension.CustomField, error) {
	ctx, err := f.core.beginRead(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(customFieldObjects, object) {
		return nil, fmt.Errorf("%w: custom fields are listed for %v", extension.ErrInvalid, customFieldObjects)
	}
	// The columns alone are schema, but the options are catalogue content. A
	// list the caller may not open in the product is not one a unit shows them.
	if err := auth.Require(ctx, "custom_field", principal.ActionRead); err != nil {
		return nil, portRefusal(err)
	}
	cols, err := f.core.catalog().ActiveColumns(ctx, object)
	if err != nil {
		return nil, portRefusal(err)
	}
	out := make([]extension.CustomField, len(cols))
	for i, c := range cols {
		out[i] = extension.CustomField{Object: object, Name: c.Name, Type: c.Type, Options: c.Options}
	}
	return out, nil
}
