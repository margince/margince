// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity's custom-field values on the way out, and in a patch. Every
// exported read that answers an activity fills them here, held by
// TestEveryReturnedActivityCarriesItsCustomFieldValues.

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CustomColumns is the catalog's answer, read by a composite page before it
// opens its transaction. The columns are unexported so no caller can widen a
// read to a column of its own choosing.
type CustomColumns struct{ cols []fieldcatalog.Column }

// ActiveActivityColumns reads the active activity custom fields. Which columns
// exist is schema, and the read that uses them carries its own gate.
func (s *Store) ActiveActivityColumns(ctx context.Context) (CustomColumns, error) {
	cols, err := s.customColumns(ctx)
	return CustomColumns{cols: cols}, err
}

// ActivityColumnsFrom is ActiveActivityColumns through a catalog the caller
// bound to the transaction it already holds, so reading it takes no second
// connection.
func ActivityColumnsFrom(ctx context.Context, catalog fieldcatalog.Reader) (CustomColumns, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return CustomColumns{}, err
	}
	cols, err := catalog.ActiveColumns(ctx, customFieldObject)
	return CustomColumns{cols: cols}, err
}

// attachCustomValues fills the activities a store method is about to return,
// after its own transaction has committed.
func (s *Store) attachCustomValues(ctx context.Context, rows ...*crmcontracts.Activity) error {
	active, err := s.customColumns(ctx)
	if err != nil || len(active) == 0 {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error { return withCustomValues(ctx, tx, active, rows) })
}

// pageOf points at every activity of a page, so one fill writes into it.
func pageOf(page []crmcontracts.Activity) []*crmcontracts.Activity {
	out := make([]*crmcontracts.Activity, len(page))
	for i := range page {
		out[i] = &page[i]
	}
	return out
}

// withCustomValues fills rows with their custom-field values in one query. A
// withheld or held row gets none: the values are content, like its subject.
func withCustomValues(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, rows []*crmcontracts.Activity) error {
	if len(active) == 0 {
		return nil
	}
	readable := make([]ids.UUID, 0, len(rows))
	at := make(map[ids.UUID][]*crmcontracts.Activity, len(rows))
	for _, a := range rows {
		if a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		id := ids.UUID(a.Id)
		if _, seen := at[id]; !seen {
			readable = append(readable, id)
		}
		at[id] = append(at[id], a)
	}
	if len(readable) == 0 {
		return nil
	}
	found, err := tx.Query(ctx,
		`SELECT id`+storekit.SelectSuffix(active)+` FROM activity WHERE id = ANY($1) AND restricted_at IS NULL`, readable)
	if err != nil {
		return err
	}
	defer found.Close()
	for found.Next() {
		var id ids.UUID
		dests := storekit.ScanDests(active)
		if err := found.Scan(append([]any{&id}, dests...)...); err != nil {
			return err
		}
		values := storekit.ExtractValues(active, dests)
		for _, a := range at[id] {
			a.AdditionalProperties = values
		}
	}
	return found.Err()
}

// withCustomValuesOf is withCustomValues for one activity.
func withCustomValuesOf(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, a *crmcontracts.Activity) error {
	return withCustomValues(ctx, tx, active, []*crmcontracts.Activity{a})
}

// customFieldImage adds the active custom fields to one side of an update's
// audit image. A patched value then shows in the diff like a core column.
func customFieldImage(image map[string]any, a crmcontracts.Activity, active []fieldcatalog.Column) map[string]any {
	for _, c := range active {
		image[c.Name] = a.AdditionalProperties[c.Name]
	}
	return image
}

// customAssignments renders the patch's `, "cf_x" = $n` assignments, binding
// each value through arg. A null clears the column. storekit.CheckCustomValues has
// already refused every key and shape this would otherwise drop.
func customAssignments(active []fieldcatalog.Column, values map[string]any, arg func(any) int) string {
	var b strings.Builder
	for _, c := range active {
		v, present := values[c.Name]
		if !present {
			continue
		}
		var bound any
		if sv, ok := storekit.SQLValue(c, v); ok {
			bound = sv
		}
		b.WriteString(", " + pgx.Identifier{c.Name}.Sanitize() + " = $" + strconv.Itoa(arg(bound)))
	}
	return b.String()
}

// changesCustom is true when the patch sets a custom field to a value the row
// does not hold, or clears one that holds a value.
func (in UpdateActivityInput) changesCustom(current crmcontracts.Activity) bool {
	p := storekit.NewPatch()
	for name, v := range in.CustomFields {
		p.Set(name, current.AdditionalProperties[name], v)
	}
	return !p.Empty()
}

// ErasableColumns holds the activity custom-field columns an erase empties,
// retired ones included, read before the erasing transaction opens.
type ErasableColumns struct{ cols []fieldcatalog.Column }

// ErasableActivityColumns reads the columns an erase must empty. A store with
// no catalog wired refuses, because it cannot tell which content it would leave.
func (s *Store) ErasableActivityColumns(ctx context.Context) (ErasableColumns, error) {
	if s.catalog == nil {
		return ErasableColumns{}, errors.New("activities: this store has no custom-field catalog, so it cannot erase activity custom-field values")
	}
	cols, err := s.catalog.FilterableColumns(ctx, customFieldObject)
	return ErasableColumns{cols: cols}, err
}

// contentTests renders the columns as an `OR "cf_x" IS NOT NULL` test of
// whether a row holds a value, and as the `, "cf_x" = NULL` assignments that empty it.
func (e ErasableColumns) contentTests() (held, clears string) {
	var h, c strings.Builder
	for _, col := range e.cols {
		name := pgx.Identifier{col.Name}.Sanitize()
		h.WriteString(" OR " + name + " IS NOT NULL")
		c.WriteString(", " + name + " = NULL")
	}
	return h.String(), c.String()
}
