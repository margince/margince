// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension/crm"
)

// The read half of Core. Every verb answers what the caller may see, under the
// same object grant, row scope and field masks as the product's own list. A
// record outside that scope answers ErrNotFound, as a missing one does. A job
// tick or a bus delivery has no caller, so each verb answers ErrForbidden to
// one. Those read through Extension.SystemReads instead.

// MaxReadPage is the most records one List returns. A larger Limit is cut to it.
const MaxReadPage = 100

// ReadPage is the paging every List takes.
type ReadPage struct {
	// Cursor is the NextCursor of the page before; empty asks for the first.
	Cursor string
	// Limit is how many records to return; zero means the default of 50.
	Limit int
}

// Page is one page of a List.
type Page[T any] struct {
	Items []T
	// NextCursor is empty on the last page.
	NextCursor string
}

// DealQuery narrows a deal list. Every field is optional, and all that are set apply.
type DealQuery struct {
	ReadPage
	// Search matches the deal's name.
	Search    string
	StageID   string
	OwnerID   string
	CompanyID string
	// Status is open, won or lost.
	Status string
	// SourceSystem and SourceID find the deal that holds one id from another
	// system. Both or neither.
	SourceSystem string
	SourceID     string
	// CustomFields matches custom fields by value, keyed by the field's column
	// name (`cf_region`). An unknown name answers ErrInvalid.
	CustomFields    map[string]string
	IncludeArchived bool
}

// CompanyQuery narrows a company list. Every field is optional, and all that are set apply.
type CompanyQuery struct {
	ReadPage
	// Search matches the company's name.
	Search  string
	OwnerID string
	// Domain finds the company that lists the domain.
	Domain       string
	Lifecycle    string
	SourceSystem string
	SourceID     string
	// CustomFields is as on DealQuery.
	CustomFields    map[string]string
	IncludeArchived bool
}

// SignalQuery narrows a signal list. Every field is optional, and all that are set apply.
type SignalQuery struct {
	ReadPage
	Kind            string
	Status          string
	ResolutionState string
	// CompanyID keeps signals about the company or one of its contacts.
	CompanyID       string
	IncludeArchived bool
}

// ActivityQuery narrows an activity list. Every field is optional, and all that are set apply.
type ActivityQuery struct {
	ReadPage
	Kind string
	// Search matches the activity's text.
	Search string
	// EntityType and EntityID keep the activities linked to one record. Both or neither.
	EntityType string
	EntityID   string
	// CustomFields is as on DealQuery.
	CustomFields    map[string]string
	IncludeArchived bool
}

// CustomField is one field an administrator added to a core record type.
type CustomField struct {
	// Object is the record type: `deal`, `company` or `activity`.
	Object string
	// Name is the key the field's value carries on the record and in a query's
	// CustomFields.
	Name string
	// Type is text, number, date, currency, picklist, multiselect or boolean.
	Type    string
	Options []string
}

// DealRepo reads deals.
type DealRepo interface {
	// Get answers ErrInvalid for an id that is not a UUID, and ErrNotFound for a
	// deal that is missing, archived or outside the caller's scope.
	Get(ctx context.Context, id string) (crm.Deal, error)
	List(ctx context.Context, q DealQuery) (Page[crm.Deal], error)
}

// CompanyRepo reads companies.
type CompanyRepo interface {
	Get(ctx context.Context, id string) (crm.Company, error)
	List(ctx context.Context, q CompanyQuery) (Page[crm.Company], error)

	// SetExtensionTypes replaces the types the unit holds on the company, by the
	// short names in Extension.CompanyTypes. Other units' types stay, and an empty
	// list clears the unit's own. An undeclared name is ErrInvalid. A scheduled
	// job gets ErrForbidden, as for any core write.
	SetExtensionTypes(ctx context.Context, id string, types []string) (crm.Company, error)
}

// SignalRepo reads signals.
type SignalRepo interface {
	Get(ctx context.Context, id string) (crm.Signal, error)
	List(ctx context.Context, q SignalQuery) (Page[crm.Signal], error)
}

// CustomFieldRepo reads which custom fields exist.
type CustomFieldRepo interface {
	// List answers the active fields of one record type, or ErrInvalid for a
	// type that has none.
	List(ctx context.Context, object string) ([]CustomField, error)
}

// Tombstone is all a read of an erased company or deal may answer: the id that
// was and the kind of record it named. It has no other field, so nothing the
// erasure removed can come back through it.
type Tombstone struct {
	ID string `json:"id"`
	// Kind is company or deal.
	Kind string `json:"kind"`
}
