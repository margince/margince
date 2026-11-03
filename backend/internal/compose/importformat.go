// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The import format: which files an import takes, in which order, and each
// column with its guide. The template, the screen's column guide and the
// agent's description are all read from buildImportFormat.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importguide"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// GetImportFormat answers the format in the language asked for, else the
// installation's. The query-enum check has refused any other lang already.
func (h importHandlers) GetImportFormat(w http.ResponseWriter, r *http.Request, params crmcontracts.GetImportFormatParams) {
	lang := installationLanguage(h.db.Pool())(r.Context())
	if params.Lang != nil {
		lang = textlang.Lang(*params.Lang)
	}
	limitMB, err := h.uploadLimitMB()
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	f, err := buildImportFormat(r.Context(), h.db, lang, limitMB)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, f)
}

// uploadLimitMB is the upload ceiling in the whole MB the upload's refusal
// counts in, as the format states it. A
// format with no ceiling would tell the reader nothing may be uploaded, so it
// is refused as the upload is.
func (h importHandlers) uploadLimitMB() (int, error) {
	if h.uploadLimit <= 0 {
		return 0, errUploadLimitUnset
	}
	return int(httperr.WholeMegabytes(h.uploadLimit)), nil
}

// buildImportFormat is the format for this workspace in one language. It
// reads only the custom-field catalog, never the create columns, so a caller
// who may read imports but not create records still gets it.
func buildImportFormat(ctx context.Context, db *database.DB, lang textlang.Lang, capMB int) (crmcontracts.ImportFormat, error) {
	if err := auth.Require(ctx, importRunGrant, principal.ActionRead); err != nil {
		return crmcontracts.ImportFormat{}, err
	}
	cats, err := readImportCatalogs(ctx, db, migration.ImportOrder)
	if err != nil {
		return crmcontracts.ImportFormat{}, err
	}
	return formatFrom(cats, langcopy.For(string(lang)), capMB)
}

// formatFrom is the format built from catalogs already read.
func formatFrom(cats map[string]importCatalog, spoken langcopy.Spoken, capMB int) (crmcontracts.ImportFormat, error) {
	f := crmcontracts.ImportFormat{
		Version:     crmcontracts.ImportFormatVersionMarginceImport1,
		Order:       make([]crmcontracts.ImportObject, 0, len(migration.ImportOrder)),
		Files:       make([]crmcontracts.ImportFormatFile, 0, len(migration.ImportOrder)),
		KeyRules:    make([]string, 0, len(importguide.KeyRules)),
		SizeLimitMb: capMB,
	}
	for _, rule := range importguide.KeyRules {
		f.KeyRules = append(f.KeyRules, spoken.Say(rule))
	}
	for _, object := range migration.ImportOrder {
		columns, err := formatColumns(object, cats[object], spoken)
		if err != nil {
			return crmcontracts.ImportFormat{}, err
		}
		f.Order = append(f.Order, crmcontracts.ImportObject(object))
		f.Files = append(f.Files, crmcontracts.ImportFormatFile{
			File: migration.FileNameFor(object), Object: crmcontracts.ImportObject(object), Columns: columns,
		})
	}
	return f, nil
}

// formatColumns is one object's columns in importTargets order: its core
// targets, then its custom fields.
func formatColumns(object string, cat importCatalog, spoken langcopy.Spoken) ([]crmcontracts.ImportFormatColumn, error) {
	targets, err := importTargets(object, cat)
	if err != nil {
		return nil, err
	}
	columns := make([]crmcontracts.ImportFormatColumn, 0, len(targets))
	for _, target := range targets {
		if col, custom := cat.byName[target]; custom {
			// A custom field's key and type are catalog data, so a caller who
			// may not read custom fields is shown none of them.
			if cat.optionsReadable {
				columns = append(columns, customFormatColumn(col, cat, spoken))
			}
			continue
		}
		guide, ok := guideFor(object, target)
		if !ok {
			return nil, fmt.Errorf("import format: %s.%s has no guide", object, target)
		}
		column := crmcontracts.ImportFormatColumn{
			Name: target, Description: spoken.Say(guide.Description), Example: spoken.Say(guide.Example),
			RequiredOnNewRow: guide.RequiredOnNew,
		}
		fieldLabel := spoken.Say(guide.Label)
		column.FieldLabel = &fieldLabel
		if object == migration.ObjectLead && target == leadTargetStatus {
			options := contacts.OpenLeadStatuses()
			column.Options = &options
		}
		columns = append(columns, column)
	}
	return columns, nil
}

// customFormatColumn is a custom field by its key, with its label and options.
func customFormatColumn(col fieldcatalog.Column, cat importCatalog, spoken langcopy.Spoken) crmcontracts.ImportFormatColumn {
	guide := importguide.Custom[col.Type]
	out := crmcontracts.ImportFormatColumn{
		Name: col.Name, Custom: true, Type: &col.Type,
		Description: spoken.Say(guide.Description), Example: spoken.Say(guide.Example),
	}
	if label, ok := cat.labels[col.Name]; ok {
		out.Label = &label
	}
	if len(col.Options) > 0 {
		options := append([]string(nil), col.Options...)
		out.Options = &options
		out.Example = optionExample(col)
	}
	return out
}

// guideFor is the guide of one object's column: the object's own entry
// ("company.display_name") before the shared one.
func guideFor(object, target string) (importguide.Column, bool) {
	if c, ok := importguide.Columns[object+"."+target]; ok {
		return c, true
	}
	c, ok := importguide.Columns[target]
	return c, ok
}

// optionExample is a value the field takes: its first option, or its first
// two for a multiselect.
func optionExample(col fieldcatalog.Column) string {
	if col.Type == fieldcatalog.TypeMultiselect && len(col.Options) > 1 {
		return strings.Join(col.Options[:2], "; ")
	}
	return col.Options[0]
}
