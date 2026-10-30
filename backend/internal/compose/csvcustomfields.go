// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A workspace's custom fields as import targets: offered by key and suggested
// by label or key. The caller-opened creates carry them on the columns read
// before the row transaction opens.

import (
	"context"
	"errors"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// importCatalog is one object's active custom fields, keyed by column name,
// and their labels. It needs no create grant, so profile, stage and format can
// read it for a caller who may not write records. Options always check a
// cell; optionsReadable says whether the fields' keys, types and options may be
// shown to the caller.
type importCatalog struct {
	byName          map[string]fieldcatalog.Column
	labels          map[string]string
	optionsReadable bool
}

// importColumns is the catalog beside the store's own answer, which only the
// writers read: Create*Tx takes it from the caller.
type importColumns struct {
	catalog importCatalog
	store   contacts.CustomColumns
	// deals is the deals store's half, read for a deal part only.
	deals deals.CustomColumns
}

// readImportCatalog reads one object's catalog. An object no import lands has
// none, and importTargets refuses it.
func readImportCatalog(ctx context.Context, db *database.DB, object string) (importCatalog, error) {
	if _, importable := csvTargets[object]; !importable || !hasCustomFields(object) {
		return importCatalog{}, nil
	}
	catalog := customfields.NewService(db.Pool(), nil)
	cols, err := catalog.ActiveColumns(ctx, object)
	if err != nil {
		return importCatalog{}, err
	}
	labels, err := catalogLabels(catalog)(ctx, object)
	if err != nil {
		return importCatalog{}, err
	}
	byName := make(map[string]fieldcatalog.Column, len(cols))
	for _, col := range cols {
		byName[col.Name] = col
	}
	readable := auth.Allows(ctx, "custom_field", principal.ActionRead)
	return importCatalog{byName: byName, labels: labels, optionsReadable: readable}, nil
}

// readImportCatalogs reads each object's catalog once.
func readImportCatalogs(ctx context.Context, db *database.DB, objects []string) (map[string]importCatalog, error) {
	out := make(map[string]importCatalog, len(objects))
	for _, object := range objects {
		if _, read := out[object]; read {
			continue
		}
		cat, err := readImportCatalog(ctx, db, object)
		if err != nil {
			return nil, err
		}
		out[object] = cat
	}
	return out, nil
}

// readImportColumns is readImportCatalog plus the store's columns for one
// part's creates. A seat without the create grant reads no store columns: its
// creates are refused by that same grant before any value could be dropped.
func readImportColumns(ctx context.Context, db *database.DB, object string) (importColumns, error) {
	cat, err := readImportCatalog(ctx, db, object)
	if err != nil {
		return importColumns{}, err
	}
	if !hasCustomFields(object) {
		return importColumns{catalog: cat}, nil
	}
	store, err := newContactsStore(db).WithFieldCatalog(customfields.NewService(db.Pool(), nil)).
		ActiveCreateColumns(ctx, object)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return importColumns{}, err
	}
	out := importColumns{catalog: cat, store: store}
	if object == migration.ObjectDeal {
		if out.deals, err = dealsStoreOver(db).ActiveDealColumns(ctx); err != nil {
			return importColumns{}, err
		}
	}
	return out, nil
}

// dealsStoreOver is the deals store an import's deal rows are read and
// written through, with the custom-field catalog wired as the HTTP surface
// wires it.
func dealsStoreOver(db *database.DB) *deals.Store {
	return deals.NewStore(db, DealsInstallation()).WithFieldCatalog(customfields.NewService(db.Pool(), nil))
}

// requestedObjects answers the objects a create request names, in its order.
func requestedObjects(req crmcontracts.CreateImportRunRequest) []string {
	if req.Object != nil {
		return []string{string(*req.Object)}
	}
	if req.Parts == nil {
		return nil
	}
	objects := make([]string, 0, len(*req.Parts))
	for _, part := range *req.Parts {
		objects = append(objects, string(part.Object))
	}
	return objects
}

// commonHeaders are the headers most exports give a field under another name.
// Each type has one of a header's targets: a company is named by display_name,
// a contact or lead by full_name.
var commonHeaders = map[string][]string{
	acceptFieldName:           {fieldDisplayName, fieldFullName},
	string(recordTypeCompany): {companyNameField},
}

// suggestImportMapping is migration.SuggestMapping plus a header equal to a
// custom field's label or a common header, folded and trimmed. A field two
// headers reach is suggested for neither, since staging refuses two columns
// onto one field.
func suggestImportMapping(p migration.Profile, targets []string, cat importCatalog) map[string]string {
	suggested := migration.SuggestMapping(p, targets)
	byLabel := map[string]string{}
	for name, label := range cat.labels {
		if _, active := cat.byName[name]; !active {
			continue
		}
		folded := strings.ToLower(strings.TrimSpace(label))
		if _, shared := byLabel[folded]; shared {
			byLabel[folded] = ""
			continue
		}
		byLabel[folded] = name
	}
	headersOf := map[string][]string{}
	for header, target := range suggested {
		headersOf[target] = append(headersOf[target], header)
	}
	for _, col := range p.Columns {
		if _, taken := suggested[col.Header]; taken {
			continue
		}
		folded := strings.ToLower(strings.TrimSpace(col.Header))
		if name := byLabel[folded]; name != "" {
			headersOf[name] = append(headersOf[name], col.Header)
			continue
		}
		for _, target := range commonHeaders[folded] {
			if slices.Contains(targets, target) {
				headersOf[target] = append(headersOf[target], col.Header)
			}
		}
	}
	out := make(map[string]string, len(headersOf))
	for target, headers := range headersOf {
		if len(headers) == 1 {
			out[headers[0]] = target
		}
	}
	return out
}

// customFieldsFrom picks the checked custom-field values of the targets in
// fields. Those are the whole row on a create and the changed cells on an
// update. It is nil when there are none, which a caller-opened create reads
// as no values.
func customFieldsFrom(fields map[string]string, typed map[string]any) map[string]any {
	var out map[string]any
	for target, value := range typed {
		if _, given := fields[target]; !given || !datasource.IsCustomFieldKey(target) {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[target] = value
	}
	return out
}
