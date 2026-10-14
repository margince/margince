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
	stages, err := readDealStages(r.Context(), h.db)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := writeImportTemplate(&buf, f, langcopy.For(string(lang)), stages); err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeImportDownload(w, r, httperr.Download{ContentType: zipContentType, Filename: "margince-import.zip"}, buf.Bytes())
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
// CSV per file holding only its header row. The stages are the ones the deal
// rows are checked against, so the README names what a row may say.
func writeImportTemplate(w io.Writer, f crmcontracts.ImportFormat, spoken langcopy.Spoken, stages dealStages) error {
	archive := zip.NewWriter(w)
	if err := writeZipEntry(archive, "README.txt", []byte(templateReadme(f, spoken, stages))); err != nil {
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
		cells = append(cells, columnHeading(col))
	}
	var buf bytes.Buffer
	out := csv.NewWriter(&buf)
	if err := out.Write(cells); err != nil {
		return nil, fmt.Errorf("import template: %s: %w", file.File, err)
	}
	out.Flush()
	return buf.Bytes(), out.Error()
}

// columnHeading is the header a column carries in the template: its label,
// else its name.
func columnHeading(col crmcontracts.ImportFormatColumn) string {
	if col.Label != nil {
		return *col.Label
	}
	return col.Name
}

// templateReadme lists the order, the key rules, every allowed value, the
// pipelines with their stages, the size limit and the formula warning, then
// each file's columns and guides.
func templateReadme(f crmcontracts.ImportFormat, spoken langcopy.Spoken, stages dealStages) string {
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
	writePipelines(line, spoken, stages)
	line(0, fmt.Sprintf(spoken.Say(importguide.Template.SizeLimit), f.SizeLimitMb))
	line(0, "")
	line(0, spoken.Say(importguide.Template.Formulas))
	for _, file := range f.Files {
		line(0, "")
		line(0, file.File)
		for _, col := range file.Columns {
			name := columnHeading(col)
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
			line(2, columnHeading(col)+": "+strings.Join(*col.Options, ", "))
		}
	}
	if heading {
		line(0, "")
	}
}

// writePipelines lists every live pipeline with its stages in order, so a
// deal row can name them. It writes nothing when the reader may not read
// pipelines or none exists.
func writePipelines(line func(int, string), spoken langcopy.Spoken, stages dealStages) {
	if !stages.readable || len(stages.names) == 0 {
		return
	}
	line(0, spoken.Say(importguide.Template.Pipelines))
	for _, name := range stages.names {
		p := stages.byPipeline[foldName(name)]
		line(1, p.name+": "+strings.Join(p.names, ", "))
	}
	line(0, "")
}
