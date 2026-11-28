// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A workspace's custom fields as import targets: offered by key and suggested
// by label or key. The caller-opened creates carry them on the columns read
// before the row transaction opens.

import (
	"context"
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importcells"
	"github.com/margince/margince/backend/internal/compose/importguide"
	"github.com/margince/margince/backend/internal/compose/importrecords"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	// records is a project part's half.
	records importrecords.Columns
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
	if importrecords.Is(object) {
		records, err := importrecords.ReadColumns(ctx, importrecords.Stores{Projects: ProjectsStoreOver(db)}, object)
		return importColumns{catalog: cat, records: records}, err
	}
	if !hasCustomFields(object) {
		return importColumns{catalog: cat}, nil
	}
	// Missing create authority leaves row-level permission warnings; it does not
	// prevent an owner from committing or undoing the import.
	var store contacts.CustomColumns
	columnsCtx, err := auth.Unrecorded(ctx, object, principal.ActionCreate)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return importColumns{}, err
	}
	if err == nil {
		store, err = newContactsStore(db).WithFieldCatalog(customfields.NewService(db.Pool(), nil)).
			ActiveCreateColumns(columnsCtx, object)
		if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return importColumns{}, err
		}
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

// suggestImportMapping is migration.SuggestMapping plus a header that names a
// field another way: a custom field's label, or a core field's label or alias.
// A field two headers reach is suggested for neither, since staging refuses two
// columns onto one field.
func suggestImportMapping(p migration.Profile, object string, targets []string, cat importCatalog) map[string]string {
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
	coreNames := importguide.HeaderNames(object, targets, migration.NormalizeFieldName)
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
		if target := coreNames[migration.NormalizeFieldName(col.Header)]; target != "" {
			headersOf[target] = append(headersOf[target], col.Header)
		}
	}
	out := make(map[string]string, len(headersOf))
	for target, headers := range headersOf {
		fits, guarded := sampleGuards[target]
		if len(headers) == 1 && (!guarded || importcells.SamplesFit(p, headers[0], fits)) {
			out[headers[0]] = target
		}
	}
	return out
}

// sampleGuards are the targets a header names but a value can refuse. Another
// CRM's "ID" names no Margince record, and a headcount of plain numbers is no
// size band, so either would skip every row.
var sampleGuards = map[string]func(string) bool{
	csvTargetID:   importcells.IsMarginceID,
	fieldSizeBand: importcells.IsSizeBand,
}

// suggestionsByObject is the mapping suggested for each of objects, keyed by object.
func suggestionsByObject(
	p migration.Profile, cats map[string]importCatalog, objects []string,
) (map[string]map[string]string, error) {
	out := make(map[string]map[string]string, len(objects))
	for _, object := range objects {
		targets, err := importTargets(object, cats[object])
		if err != nil {
			return nil, err
		}
		out[object] = suggestImportMapping(p, object, targets, cats[object])
	}
	return out, nil
}

// customFieldsFrom picks the checked custom-field values of the targets in
// fields.
var customFieldsFrom = importrecords.CustomFieldsFrom
