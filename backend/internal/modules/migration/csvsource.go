// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// The objects a delimited file may carry.
//
// `lead` and `contact` are two answers to one question about a file of humans,
// and the caller picks per run. `lead` stays the right answer for a
// machine-sourced list — those rows land unworked and a human promotes the ones
// worth keeping. `contact` is for a file the business already knows: a migration
// off another CRM, a corrected export coming back, a customer list from a system
// being retired. Routing those through the lead table would force a human to
// re-approve records nobody doubts.
//
// `contact` bypasses nothing. It runs the same identity ladder every other contact
// create runs, and an email already belonging to someone else refuses the row.
const (
	ObjectLead     = "lead"
	ObjectCompany  = "company"
	ObjectContact  = "contact"
	ObjectDeal     = "deal"
	ObjectActivity = "activity"
	// ConnectorCSV is the direct migrate-in connector this source serves
	// (UC-E11-03). The column's CHECK also admits salesforce — the DDL is the
	// chapter's pinned arrival shape — but it gets its Go constant when that
	// connector lands, not before.
	ConnectorCSV = "csv"
)

// ErrObjectNotInSource reports a read for an object this file does not carry.
// One upload is one object; the engine asks per object because a Source may
// span several, and answering rows from the wrong one would import a file into
// a table nobody chose.
var ErrObjectNotInSource = errors.New("object is not in this import source")

// SkippedLine is one row the source could not deliver, named by the line a
// human would open the file to.
type SkippedLine struct {
	Line   int
	Reason string
	Why    Reason
}

// CSVSource reads an uploaded delimited file out of the blobstore as a
// migration Source.
//
// The bytes are durable rather than streamed. Source.Rows pages by absolute
// offset and a resume starts at one, so the file is read again long after its
// upload. The file's own order is the ordering the resume contract requires.
//
// One source reads its stored file once and keeps the bytes. A page that starts
// where the last one ended reads on from there. Any other offset walks from the
// top.
type CSVSource struct {
	blobs     blobstore.Store
	key       string
	object    string
	mapping   map[string]string
	sourceKey string
	// zip, when it names an entry, makes the bytes one file of a stored ZIP
	// instead of the blob itself.
	zip ZipPart

	// body is the stored file's bytes, read on first use.
	body []byte
	// rows is the file's row count once Counts has walked it; body is fixed by then.
	rows *int
	// cursor is where the last page of Rows stopped.
	cursor *rowCursor

	// skipped accumulates rows no page could deliver. One source is walked by
	// one request — the engine pages sequentially and the prediction pass runs
	// before it — so this needs no lock, and giving it one would suggest a
	// concurrency the rest of the type does not actually support.
	skipped []SkippedLine

	// observe hears where each page of Rows ended; nil hears nothing.
	observe RowsRead
	// shape rewrites each row's mapped cells before any reader sees them; nil
	// keeps the cells as the file wrote them.
	shape RowShape
}

// RowShape rewrites one row's mapped cells in place, keyed by target.
type RowShape func(fields map[string]any)

// ShapeRows sets what rewrites each row's cells, so every reader of the row
// (the dry run, the review, the commit) sees one spelling of it.
func (s *CSVSource) ShapeRows(shape RowShape) { s.shape = shape }

// RowsRead hears that a source has delivered its object's rows up to the
// given position. An error it answers ends the read with that error.
type RowsRead func(ctx context.Context, object string, delivered int) error

// ObserveRows sets what hears each page of Rows end, so a reader can report
// how far a walk of the file has come.
func (s *CSVSource) ObserveRows(observe RowsRead) { s.observe = observe }

// NewCSVSource binds one uploaded file to one object and one mapping.
// sourceKeyColumn names the column that identifies a row for idempotent upsert
// and for undo; the caller decides which column that is, so exactly one place
// makes that choice.
func NewCSVSource(blobs blobstore.Store, key, object string, mapping map[string]string, sourceKeyColumn string) *CSVSource {
	return &CSVSource{blobs: blobs, key: key, object: object, mapping: mapping, sourceKey: sourceKeyColumn}
}

// NewZipPartSource binds one entry of a stored ZIP to one object and mapping.
func NewZipPartSource(blobs blobstore.Store, part ZipPart, object string, mapping map[string]string, sourceKeyColumn string) *CSVSource {
	s := NewCSVSource(blobs, part.Key, object, mapping, sourceKeyColumn)
	s.zip = part
	return s
}

// Objects fixes the import order. One upload is one object, so there is one.
func (s *CSVSource) Objects() []string { return []string{s.object} }

// Counts reports how many rows the file holds, for the run's progress figure.
func (s *CSVSource) Counts(ctx context.Context) (map[string]int, error) {
	if s.rows == nil {
		rows := 0
		err := s.walk(ctx, func(int, []string, map[string]int) error {
			rows++
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.rows = &rows
	}
	return map[string]int{s.object: *s.rows}, nil
}

// Skipped reports the rows no page could deliver, with the line to open and the
// reason. Never silent: a file half-ignored under a success message is worse
// than a refusal.
func (s *CSVSource) Skipped() []SkippedLine {
	return append([]SkippedLine(nil), s.skipped...)
}

// Rows pages the file in its own order. offset is absolute over DELIVERABLE
// rows, which is what the engine checkpoints against.
func (s *CSVSource) Rows(ctx context.Context, object string, offset, limit int) ([]Row, error) {
	if object != s.object {
		return nil, fmt.Errorf("%w: this source carries %q, not %q", ErrObjectNotInSource, s.object, object)
	}
	if s.cursor == nil || s.cursor.delivered > offset {
		cursor, err := s.openCursor(ctx)
		if err != nil {
			return nil, err
		}
		s.cursor = cursor
	}
	out := make([]Row, 0, limit)
	for len(out) < limit {
		row, ok, err := s.cursor.next(s)
		if err != nil {
			s.cursor = nil
			return nil, err
		}
		if !ok {
			break
		}
		if s.cursor.delivered > offset {
			out = append(out, row)
		}
	}
	if s.observe != nil {
		if err := s.observe(ctx, s.object, s.cursor.delivered); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// rowCursor is one walk of the file, positioned after the last row it
// delivered.
type rowCursor struct {
	file  openFile
	index map[string]int
	// claimed holds the source keys the walk has met, with their lines. It is the
	// walk's own, because a walk sees every row before the one it serves.
	claimed   map[string]int
	delivered int
	done      bool
}

func (s *CSVSource) openCursor(ctx context.Context) (*rowCursor, error) {
	file, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	return &rowCursor{file: file, index: headerIndex(file.header), claimed: map[string]int{}}, nil
}

// next is the walk's next deliverable row, or false at the end of the file.
func (c *rowCursor) next(s *CSVSource) (Row, bool, error) {
	for !c.done {
		line, record, err := c.file.next()
		if errors.Is(err, io.EOF) {
			c.done = true
			break
		}
		if err != nil {
			return Row{}, false, err
		}
		row, ok := s.rowFrom(line, record, c.index)
		if !ok {
			continue
		}
		// A source key is a row's IDENTITY — what re-import matches on and what
		// undo finds a created row by. Two rows claiming one identity is a
		// question the file has to answer, not one the importer can: which of
		// them is the record, and what should a re-import of the other do?
		//
		// Accepting them looked harmless and was not. The identity map holds one
		// binding per key, so the second row silently reconciled onto the first
		// row's record; the report deduplicated by the same key, so two refusals
		// arrived as one skip and a phantom `unchanged`, and the four counts
		// stopped summing to rows_read.
		if first, seen := c.claimed[row.ExternalID]; seen {
			s.skip(line, Why(ReasonRowKeyRepeated,
				"column", s.sourceKey, "value", row.ExternalID, "line", strconv.Itoa(first)))
			continue
		}
		c.claimed[row.ExternalID] = line
		c.delivered++
		return row, true, nil
	}
	return Row{}, false, nil
}

// identityOf is the spelling a row's key travels under: an email folds as the
// contact store keys it, any other key is kept.
func (s *CSVSource) identityOf(external string) string {
	if s.mapping[s.sourceKey] == "email" {
		return correspondence.Fold(external)
	}
	return external
}

// rowFrom builds one Row from one record, or reports why it cannot. A row with
// no value in the source-key column can neither be recognized on a re-run nor
// found by an undo, so it is disclosed rather than landed under an invented id.
func (s *CSVSource) rowFrom(line int, record []string, index map[string]int) (Row, bool) {
	fields := make(map[string]any, len(s.mapping))
	for column, target := range s.mapping {
		at, ok := index[column]
		if !ok || at >= len(record) {
			continue
		}
		if value := strings.TrimSpace(record[at]); value != "" {
			fields[target] = value
		}
	}
	if s.shape != nil {
		s.shape(fields)
	}

	at, ok := index[s.sourceKey]
	external := ""
	if ok && at < len(record) {
		external = strings.TrimSpace(record[at])
	}
	if external == "" {
		s.skip(line, Why(ReasonRowKeyEmpty, "column", s.sourceKey))
		return Row{}, false
	}
	return Row{ExternalID: s.identityOf(external), Fields: fields, LastSyncedAt: time.Time{}, Line: line}, true
}

func (s *CSVSource) skip(line int, why Reason) {
	for _, already := range s.skipped {
		if already.Line == line {
			// A page re-read after a resume walks the same rows again; the
			// disclosure is about the FILE, so it is recorded once.
			return
		}
	}
	s.skipped = append(s.skipped, SkippedLine{Line: line, Reason: why.English(), Why: why})
}

// Header names the file's own columns, and reads no further.
//
// A mapping is written against these names, so a run is checked against them
// before it exists: rowFrom drops a column the header does not carry, and the
// field it was mapped onto then imports empty on every row.
func (s *CSVSource) Header(ctx context.Context) ([]string, error) {
	file, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer file.close(ctx)
	return append([]string(nil), file.header...), nil
}

// openFile is the stored source, framed: the body to close, the reader standing
// at the first data record, and the header it just read.
type openFile struct {
	body    io.ReadCloser
	records recordReader
	header  []string
	key     string
}

// open reads the header and leaves the reader at the first data record.
//
// Two callers want different amounts of one file — a run walks every row, a
// mapping check needs only the names — and both must frame it identically, or
// the check accepts a column the walk then cannot find.
func (s *CSVSource) open(ctx context.Context) (openFile, error) {
	body, err := s.openBody(ctx)
	if err != nil {
		return openFile{}, err
	}
	records, err := newRecordReader(body)
	if err != nil {
		openFile{body: body, key: s.key}.close(ctx)
		return openFile{}, err
	}
	file := openFile{body: body, records: records, key: s.key}
	header, err := file.records.Read()
	if err != nil {
		file.close(ctx)
		if errors.Is(err, io.EOF) {
			return openFile{}, fmt.Errorf("%w: the file has no header row", ErrHeaderInvalid)
		}
		return openFile{}, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
	}
	if err := validateHeader(header); err != nil {
		file.close(ctx)
		return openFile{}, err
	}
	file.header = header
	return file, nil
}

// openBody is the stored file, or the one entry of a stored ZIP, read from the
// store once per source.
func (s *CSVSource) openBody(ctx context.Context) (io.ReadCloser, error) {
	if s.body == nil {
		stored, err := s.storedBody(ctx)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(stored)
		if cerr := stored.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the import source", "key", s.key, "err", cerr)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
		}
		s.body = body
	}
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

func (s *CSVSource) storedBody(ctx context.Context) (io.ReadCloser, error) {
	if s.zip.Entry == "" {
		body, _, err := s.blobs.Get(ctx, s.key)
		if err != nil {
			return nil, fmt.Errorf("import source %q: %w", s.key, err)
		}
		return body, nil
	}
	return openZipEntry(ctx, s.blobs, s.zip)
}

// close releases the stored file. A close error cannot change what was already
// parsed, and shadowing the parse result with it would report the wrong
// failure — so it is logged rather than returned, and never dropped.
func (f openFile) close(ctx context.Context) {
	if err := f.body.Close(); err != nil {
		slog.WarnContext(ctx, "closing the import source", "key", f.key, "err", err)
	}
}

// walk opens the object and calls visit for each data record, handing it the
// file line (the header is line 1) and the header index. One reader, one place
// that knows the file's framing.
func (s *CSVSource) walk(ctx context.Context, visit func(line int, record []string, index map[string]int) error) error {
	file, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer file.close(ctx)
	index := headerIndex(file.header)
	for {
		line, record, err := file.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := visit(line, record, index); err != nil {
			return err
		}
	}
}

// headerIndex maps each column to its position, by the header as the file
// spells it. That is the key ProfileCSV published and the mapping was
// written against. Trimming here would make a header like "Email " unresolvable.
func headerIndex(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	return index
}

// next reads one data record and the file line it starts on.
func (f openFile) next() (int, []string, error) {
	record, err := f.records.Read()
	if errors.Is(err, io.EOF) {
		return 0, nil, io.EOF
	}
	// The reader's own position, not a counter. A quoted field may span
	// several lines, and the Nth record is then not line N+1.
	line, _ := f.records.FieldPos(0)
	if err != nil {
		return line, nil, fmt.Errorf("%w: line %d: %v", ErrSourceUnreadable, line, err)
	}
	return line, record, nil
}
