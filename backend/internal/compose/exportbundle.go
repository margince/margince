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
	"database/sql/driver"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
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
			cell, err := driverCell(row[i])
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

// rowsAsMaps shapes a member's rows as column→value maps for the JSON
// dump, re-embedding jsonb bytes as raw JSON (never base64) and uuids as
// their canonical strings.
func rowsAsMaps(m memberData) []map[string]any {
	out := make([]map[string]any, 0, len(m.rows))
	for _, row := range m.rows {
		rec := make(map[string]any, len(m.columns))
		for i, col := range m.columns {
			rec[col] = jsonValue(row[i])
		}
		out = append(out, rec)
	}
	return out
}

// jsonValue normalizes a driver value for the JSON dump and for a CSV cell
// that holds JSON. It walks into jsonb objects and arrays, so a uuid inside an
// array is a string there too.
//
//craft:ignore naked-any a driver row value spans every SQL type the exported tables carry; the switch narrows each
func jsonValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case [16]byte:
		return ids.UUID(t).String()
	case []byte:
		return byteaText(t)
	case pgtype.Interval:
		return valuerJSON{t}
	case pgtype.Bits:
		return valuerJSON{t}
	case []any:
		out := make([]any, len(t))
		for i, element := range t {
			out[i] = jsonValue(element)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, element := range t {
			out[key] = jsonValue(element)
		}
		return out
	default:
		return v
	}
}

// valuerJSON spells a pgtype value as its Postgres text. Interval and Bits have
// no JSON form of their own and would marshal as their struct fields.
type valuerJSON struct{ driver.Valuer }

func (v valuerJSON) MarshalJSON() ([]byte, error) {
	text, err := v.Value()
	if err != nil {
		return nil, err
	}
	return json.Marshal(text)
}

// byteaText is Postgres's hex text for a bytea, which an import reads back.
// Raw bytes are not text, and as JSON they would not parse at all.
func byteaText(b []byte) string {
	return `\x` + hex.EncodeToString(b)
}

// driverCell renders a value pgx decoded from an exported column as one CSV
// field. Numbers, intervals and bit strings keep the text Postgres prints; jsonb
// objects, arrays and SQL arrays are compact JSON, guarded like any text.
//
//craft:ignore naked-any a driver row value spans every SQL type the exported tables carry; the switch narrows each
func driverCell(v any) (string, error) {
	switch t := v.(type) {
	case pgtype.Numeric:
		return valuerText(t)
	case pgtype.Interval:
		return valuerText(t)
	case pgtype.Bits:
		return valuerText(t)
	case []any, map[string]any:
		raw, err := json.Marshal(jsonValue(t))
		if err != nil {
			return "", err
		}
		return formulacell.Guard(string(raw)), nil
	default:
		return csvCell(v), nil
	}
}

// valuerText is a pgtype value's Postgres text, or an empty cell for NULL.
func valuerText(v driver.Valuer) (string, error) {
	value, err := v.Value()
	if err != nil || value == nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("compose: %T rendered as %T, want text", v, value)
	}
	return text, nil
}

// csvCell renders a scalar as a single CSV field. Only the text branches are
// guarded: a typed negative number must keep its sign without a quote.
//
//craft:ignore naked-any a driver row value spans every SQL type the exported tables carry; the switch narrows each
func csvCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case [16]byte:
		return ids.UUID(t).String()
	case []byte:
		return byteaText(t)
	case string:
		return formulacell.Guard(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
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
