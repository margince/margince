// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The export bundle's serializer: the ZIP layout the reader (and the
// round-trip re-importer) sees — a CSV per object, one relational JSON
// dump, the files manifest, and the bundle manifest that carries the
// AC-OV-9 honest-scope disclosure. export.go decides WHAT the bundle
// contains and under whose visibility; this file decides how it is
// written down.

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/rowvalue"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writeZip packs the collected members into the bundle: a CSV per object,
// the relational JSON dump, the files manifest, and the bundle manifest.
func writeZip(dst io.Writer, actor principal.Principal, wsID ids.UUID, members []memberData, summary BundleSummary) error {
	zw := zip.NewWriter(dst)

	dump := make(map[string]any, len(members))
	var filesManifest []map[string]any
	generatedAt := time.Now().UTC()

	manifest := bundleManifest{
		Format:         exportFormat,
		WorkspaceID:    wsID.String(),
		GeneratedAt:    generatedAt,
		GeneratedBy:    actor.ID,
		OmittedObjects: summary.Omitted,
		Note: "Row-scoped to the exporting principal; open formats only (CSV per object + a relational JSON dump). " +
			"File bytes are referenced by storage_key, not embedded — see files-manifest.json.",
	}

	for _, m := range members {
		if err := writeCSV(zw, m); err != nil {
			return err
		}
		dump[m.table] = rowsAsMaps(m)
		manifest.Members = append(manifest.Members, manifestMember{
			Object: m.table, File: m.table + ".csv", Rows: len(m.rows),
		})
		if m.table == "attachment" {
			filesManifest = rowsAsMaps(m)
		}
	}

	if err := writeJSON(zw, "data.json", relationalDump{
		Format: exportFormat, GeneratedAt: generatedAt, Objects: dump,
	}); err != nil {
		return err
	}
	if err := writeJSON(zw, "files-manifest.json", filesManifestDoc{
		Note: "Every attachment with its integrity checksum. File bytes live in the object store under storage_key; " +
			"the blob substrate (B-EP02.27) is not yet wired in this build, so bytes are referenced here, not bundled.",
		Files: filesManifest,
	}); err != nil {
		return err
	}
	if err := writeJSON(zw, "manifest.json", manifest); err != nil {
		return err
	}

	return zw.Close()
}

// writeCSV writes one member as a CSV entry of the bundle.
func writeCSV(zw *zip.Writer, m memberData) error {
	f, err := zw.Create(m.table + ".csv")
	if err != nil {
		return err
	}
	return writeMemberCSV(f, m)
}

// writeMemberCSV is the one CSV spelling of a member, shared by the bundle and
// the filtered export. The derived column list is the header, and each driver
// value is one flat cell.
func writeMemberCSV(dst io.Writer, m memberData) error {
	cw := csv.NewWriter(dst)
	if err := cw.Write(m.columns); err != nil {
		return err
	}
	record := make([]string, len(m.columns))
	for _, row := range m.rows {
		for i := range m.columns {
			cell, err := rowvalue.CSVCell(row[i])
			if err != nil {
				return fmt.Errorf("export %s.%s: %w", m.table, m.columns[i], err)
			}
			record[i] = cell
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// writeJSON marshals v into a ZIP entry as indented JSON.
//
//craft:ignore naked-any the bundle documents are heterogeneous JSON shapes assembled for handover, not a single typed record
func writeJSON(zw *zip.Writer, name string, v any) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// rowsAsMaps shapes a member's rows as column→value maps for the JSON dump,
// each value normalized by rowvalue.Document.
func rowsAsMaps(m memberData) []map[string]any {
	out := make([]map[string]any, 0, len(m.rows))
	for _, row := range m.rows {
		rec := make(map[string]any, len(m.columns))
		for i, col := range m.columns {
			rec[col] = rowvalue.Document(row[i])
		}
		out = append(out, rec)
	}
	return out
}

// bundleManifest describes the bundle: format, provenance, the members
// present, and any objects the caller's grants excluded.
type bundleManifest struct {
	Format         string           `json:"format"`
	WorkspaceID    string           `json:"workspace_id,omitempty"`
	GeneratedAt    time.Time        `json:"generated_at"`
	GeneratedBy    string           `json:"generated_by"`
	Members        []manifestMember `json:"members"`
	OmittedObjects []string         `json:"omitted_objects,omitempty"`
	Note           string           `json:"note"`
}

type manifestMember struct {
	Object string `json:"object"`
	File   string `json:"file"`
	Rows   int    `json:"rows"`
}

// relationalDump is the single JSON view of every exported object.
//
//craft:ignore naked-any Objects maps each table name to its exported rows, whose columns are schema-derived at runtime
type relationalDump struct {
	Format      string         `json:"format"`
	GeneratedAt time.Time      `json:"generated_at"`
	Objects     map[string]any `json:"objects"`
}

// filesManifestDoc is the files manifest: every attachment plus a note on
// where the bytes live.
type filesManifestDoc struct {
	Note  string           `json:"note"`
	Files []map[string]any `json:"files"`
}
