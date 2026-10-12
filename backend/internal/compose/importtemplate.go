// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importguide"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// DownloadImportTemplate answers the template ZIP in the installation's
// language. It is built whole before the first byte, so a failure is answered.
func (h importHandlers) DownloadImportTemplate(w http.ResponseWriter, r *http.Request) {
	lang := installationLanguage(h.db.Pool())(r.Context())
	f, err := buildImportFormat(r.Context(), h.db, lang, h.uploadLimitMB())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := writeImportTemplate(&buf, f, langcopy.For(string(lang))); err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeImportDownload(w, r, httperr.Download{ContentType: "application/zip", Filename: "margince-import.zip"}, buf.Bytes())
}

// writeImportDownload sends a body already built whole. A failed write is a
// broken connection after the 200, so it can only be logged.
func writeImportDownload(w http.ResponseWriter, r *http.Request, d httperr.Download, body []byte) {
	d.Size = int64(len(body))
	d.WriteHeaders(w)
	if _, err := w.Write(body); err != nil { // #nosec G705 -- an attachment, never rendered as a page
		slog.WarnContext(r.Context(), "import download: the client's download is truncated", "file", d.Filename, "err", err)
	}
}

// writeImportTemplate writes the template ZIP of a format: a README and one
// CSV per file holding only its header row.
func writeImportTemplate(w io.Writer, f crmcontracts.ImportFormat, spoken langcopy.Spoken) error {
	archive := zip.NewWriter(w)
	if err := writeZipEntry(archive, "README.txt", []byte(templateReadme(f, spoken))); err != nil {
		return err
	}
	for _, file := range f.Files {
		header, err := templateHeader(file)
		if err != nil {
			return err
		}
		if err := writeZipEntry(archive, file.File, header); err != nil {
			return err
		}
	}
	return archive.Close()
}

func writeZipEntry(archive *zip.Writer, name string, body []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("import template: %s: %w", name, err)
	}
	if _, err := entry.Write(body); err != nil {
		return fmt.Errorf("import template: %s: %w", name, err)
	}
	return nil
}

// templateHeader is one file's header row, a column's label where it has one.
func templateHeader(file crmcontracts.ImportFormatFile) ([]byte, error) {
	cells := make([]string, 0, len(file.Columns))
	for _, col := range file.Columns {
		cells = append(cells, shownName(col))
	}
	var buf bytes.Buffer
	out := csv.NewWriter(&buf)
	if err := out.Write(cells); err != nil {
		return nil, fmt.Errorf("import template: %s: %w", file.File, err)
	}
	out.Flush()
	return buf.Bytes(), out.Error()
}

// shownName is what a person reads for a column: its label, else its name.
func shownName(col crmcontracts.ImportFormatColumn) string {
	if col.Label != nil {
		return *col.Label
	}
	return col.Name
}

// templateReadme lists the order, the key rules, every allowed value, the
// size limit and the formula warning, then each file's columns and guides.
func templateReadme(f crmcontracts.ImportFormat, spoken langcopy.Spoken) string {
	var b strings.Builder
	line := func(indent int, text string) {
		b.WriteString(strings.Repeat("  ", indent) + text + "\n")
	}
	line(0, fmt.Sprintf(spoken.Say(importguide.Template.Title), f.Version))
	line(0, "")
	line(0, spoken.Say(importguide.Template.Optional))
	line(0, "")
	files := make([]string, 0, len(f.Files))
	for _, file := range f.Files {
		files = append(files, file.File)
	}
	line(0, spoken.Say(importguide.Template.Order))
	line(1, strings.Join(files, ", "))
	line(0, "")
	line(0, spoken.Say(importguide.Template.Keys))
	for _, rule := range f.KeyRules {
		line(1, "* "+rule)
	}
	line(0, "")
	writeAllowedValues(line, f, spoken)
	line(0, fmt.Sprintf(spoken.Say(importguide.Template.SizeLimit), f.SizeLimitMb))
	line(0, "")
	line(0, spoken.Say(importguide.Template.Formulas))
	for _, file := range f.Files {
		line(0, "")
		line(0, file.File)
		for _, col := range file.Columns {
			name := shownName(col)
			if col.RequiredOnNewRow {
				name += " (" + spoken.Say(importguide.Template.Required) + ")"
			}
			line(1, name)
			line(2, col.Description)
			line(2, fmt.Sprintf(spoken.Say(importguide.Template.Example), col.Example))
		}
	}
	return b.String()
}

// writeAllowedValues lists each file's columns that carry options, and writes
// nothing when none does.
func writeAllowedValues(line func(int, string), f crmcontracts.ImportFormat, spoken langcopy.Spoken) {
	heading := false
	for _, file := range f.Files {
		fileNamed := false
		for _, col := range file.Columns {
			if col.Options == nil || len(*col.Options) == 0 {
				continue
			}
			if !heading {
				line(0, spoken.Say(importguide.Template.Allowed))
				heading = true
			}
			if !fileNamed {
				line(1, file.File)
				fileNamed = true
			}
			line(2, shownName(col)+": "+strings.Join(*col.Options, ", "))
		}
	}
	if heading {
		line(0, "")
	}
}
