// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importrecords is what a CSV import knows about a project and a product.
// For each it holds the file's targets and the checks of a row's cells. It also
// holds the store inputs a row becomes and the words a store refusal is reported in.
//
// The import's writers in package compose find the record, decide create or
// update, keep the ledger of keys, and open the transaction. This package is the
// part that differs by record type, so those writers hold one dispatch to it.
// Every write goes through the store that owns the record.
package importrecords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Landing writes the record one row creates, with the row's identity, tags and
// context tag, in one transaction.
type Landing func(ctx context.Context, row migration.Row, create func(tx pgx.Tx) (ids.UUID, error)) error

// Record is one record type's half of an import.
type Record interface {
	// Read is the stored record as its own JSON, as the row-scoped read sees it.
	Read(ctx context.Context, id ids.UUID) ([]byte, error)
	// Archived reports whether the record is archived. A record the caller cannot
	// see answers not found.
	Archived(ctx context.Context, id ids.UUID) (bool, error)
	// CheckCells reads the cells only this type has into typed, or answers each
	// cell the store would refuse.
	CheckCells(fields map[string]string, typed map[string]any) (map[string]any, []*CellProblem)
	// RefuseNew says why the row cannot land as a new record from its own cells.
	RefuseNew(fields map[string]string, typed map[string]any) migration.Reason
	// PredictCreate asks the store, without writing, the refusals a create meets
	// outside the row's cells.
	PredictCreate(ctx context.Context, fields map[string]string, typed map[string]any) (migration.Reason, error)
	// Create lands the row as a new record.
	Create(ctx context.Context, land Landing, row migration.Row, typed map[string]any, sourceSystem string) (migration.EnsureResult, error)
	// RefuseChange says why changing the stored record as the row asks is refused,
	// asked without writing.
	RefuseChange(current []byte, changed map[string]string, typed map[string]any) migration.Reason
	// Update writes the changed fields of the record.
	Update(ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any, current []byte, ifVersion *int64) error
	// Lock takes the row lock of the stored record and names its table, for a
	// reviewed import's version check.
	Lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error)
	// Undo archives a record the import created, unless a colleague worked on it
	// since importedAt.
	Undo(ctx context.Context, id ids.UUID, importedAt time.Time) error
	// Refusal reads a store's refusal of one row as the reason the report shows.
	// Anything it does not know is not a bad row and stops the run.
	Refusal(err error, fields map[string]string) (migration.Reason, bool)
}

// Stores are the stores the records are written through, and the pool the
// dry run's reads use.
type Stores struct {
	Pool     *pgxpool.Pool
	Projects *projects.Store
	Products *deals.Store
}

// Columns are the custom-field columns a part's creates carry, read before any
// row transaction opens because the catalog read takes a connection of its own.
type Columns struct {
	projects projects.CustomColumns
}

// ReadColumns reads the columns of the object, empty for a type with no custom
// fields.
func ReadColumns(ctx context.Context, stores Stores, object string) (Columns, error) {
	if object != migration.ObjectProject {
		return Columns{}, nil
	}
	cols, err := stores.Projects.ActiveProjectColumns(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// A seat that may not read projects may not create them either. The create
		// grant refuses its rows before a custom value could be dropped.
		return Columns{}, nil
	}
	return Columns{projects: cols}, err
}

// For is the record type of a project or product import object.
func For(stores Stores, object string, cols Columns) Record {
	if object == migration.ObjectProject {
		return projectRecord{stores: stores, cols: cols.projects}
	}
	return productRecord{stores: stores}
}

// None is the record type of an object this package does not serve. It reads
// nothing, refuses nothing and checks no cell. A writer holds one Record for
// every object and asks it without a nil check.
var None Record = none{}

// Is reports whether the object is one of these record types.
func Is(object string) bool {
	return object == migration.ObjectProject || object == migration.ObjectProduct
}

// SkippedFor is the result of a row left unwritten for why.
func SkippedFor(why migration.Reason) migration.EnsureResult {
	return migration.EnsureResult{Skipped: true, SkipReason: why.English(), SkipWhy: why}
}

// encode renders one stored record as JSON, the surface a row's cells are
// compared against.
func encode[T crmcontracts.Project | crmcontracts.Product](record T) ([]byte, error) {
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("import: reading the stored record: %w", err)
	}
	return encoded, nil
}

type none struct{}

func (none) Read(context.Context, ids.UUID) ([]byte, error)   { return nil, nil }
func (none) Archived(context.Context, ids.UUID) (bool, error) { return false, nil }
func (none) CheckCells(_ map[string]string, typed map[string]any) (map[string]any, []*CellProblem) {
	return typed, nil
}
func (none) RefuseNew(map[string]string, map[string]any) migration.Reason { return migration.Reason{} }
func (none) PredictCreate(context.Context, map[string]string, map[string]any) (migration.Reason, error) {
	return migration.Reason{}, nil
}
func (none) Create(context.Context, Landing, migration.Row, map[string]any, string) (migration.EnsureResult, error) {
	return migration.EnsureResult{}, nil
}
func (none) RefuseChange([]byte, map[string]string, map[string]any) migration.Reason {
	return migration.Reason{}
}
func (none) Update(context.Context, ids.UUID, map[string]string, map[string]any, []byte, *int64) error {
	return nil
}
func (none) Lock(context.Context, pgx.Tx, ids.UUID) (string, error) { return "", nil }
func (none) Undo(context.Context, ids.UUID, time.Time) error        { return nil }
func (none) Refusal(error, map[string]string) (migration.Reason, bool) {
	return migration.Reason{}, false
}
