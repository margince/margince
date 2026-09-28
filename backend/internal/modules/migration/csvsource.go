// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/blobstore"
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
	ObjectLead    = "lead"
	ObjectCompany = "company"
	ObjectContact = "contact"
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

// errPageFull stops a walk once the requested page is complete. Internal, and
// never returned to a caller: a page that filled is a success, and reading the
// remaining rows to discover that would make every page cost a whole file.
var errPageFull = errors.New("page full")

// SkippedLine is one row the source could not deliver, named by the line a
// human would open the file to.
type SkippedLine struct {
	Line   int
	Reason string
}

// CSVSource reads an uploaded delimited file out of the blobstore as a
// migration Source.
//
// Why the bytes are durable rather than streamed: Source.Rows pages by absolute
// offset and the engine's checkpoint resumes at one, so the file must be
// re-readable after the request that uploaded it is long gone. Re-opening and
// skipping to the offset is O(rows) per page, which the 10 MB body cap bounds —
// and the file's own order IS the deterministic ordering the resume contract
// requires, with no cursor to store or invalidate.
type CSVSource struct {
	blobs     blobstore.Store
	key       string
	object    string
	mapping   map[string]string
	sourceKey string

	// skipped accumulates rows no page could deliver. One source is walked by
	// one request — the engine pages sequentially and the prediction pass runs
	// before it — so this needs no lock, and giving it one would suggest a
	// concurrency the rest of the type does not actually support.
	skipped []SkippedLine
}

// NewCSVSource binds one uploaded file to one object and one mapping.
// sourceKeyColumn names the column that identifies a row for idempotent upsert
// and for undo; the caller decides which column that is, so exactly one place
// makes that choice.
func NewCSVSource(blobs blobstore.Store, key, object string, mapping map[string]string, sourceKeyColumn string) *CSVSource {
	return &CSVSource{blobs: blobs, key: key, object: object, mapping: mapping, sourceKey: sourceKeyColumn}
}

// Objects fixes the import order. One upload is one object, so there is one.
func (s *CSVSource) Objects() []string { return []string{s.object} }

// Counts reports how many rows the file holds, for the run's progress figure.
func (s *CSVSource) Counts(ctx context.Context) (map[string]int, error) {
	rows := 0
	err := s.walk(ctx, func(int, []string, map[string]int) error {
		rows++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]int{s.object: rows}, nil
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
	out := make([]Row, 0, limit)
	delivered := 0
	// Rebuilt per call, and every call re-walks the file from the top, so this
	// sees the whole file whatever page is being served. It is not run state and
	// must not become any: a resumed run walks the same rows again on purpose.
	claimed := map[string]int{}
	err := s.walk(ctx, func(line int, record []string, index map[string]int) error {
		row, ok := s.rowFrom(line, record, index)
		if !ok {
			return nil
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
		if first, seen := claimed[row.ExternalID]; seen {
			s.skip(line, fmt.Sprintf(
				"the %q value %q is already used by line %d; each row needs its own, because it is "+
					"what a re-import matches on and what an undo finds this row by",
				s.sourceKey, row.ExternalID, first))
			return nil
		}
		claimed[row.ExternalID] = line
		position := delivered
		delivered++
		if position < offset {
			return nil
		}
		out = append(out, row)
		if len(out) == limit {
			return errPageFull
		}
		return nil
	})
	if err != nil && !errors.Is(err, errPageFull) {
		return nil, err
	}
	return out, nil
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

	at, ok := index[s.sourceKey]
	external := ""
	if ok && at < len(record) {
		external = strings.TrimSpace(record[at])
	}
	if external == "" {
		s.skip(line, fmt.Sprintf("the %q column is empty, so this row cannot be identified for re-import or undo", s.sourceKey))
		return Row{}, false
	}
	return Row{ExternalID: external, Fields: fields, LastSyncedAt: time.Time{}, Line: line}, true
}

func (s *CSVSource) skip(line int, reason string) {
	for _, already := range s.skipped {
		if already.Line == line {
			// A page re-read after a resume walks the same rows again; the
			// disclosure is about the FILE, so it is recorded once.
			return
		}
	}
	s.skipped = append(s.skipped, SkippedLine{Line: line, Reason: reason})
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
	records *csv.Reader
	header  []string
	key     string
}

// open reads the header and leaves the reader at the first data record.
//
// Two callers want different amounts of one file — a run walks every row, a
// mapping check needs only the names — and both must frame it identically, or
// the check accepts a column the walk then cannot find.
func (s *CSVSource) open(ctx context.Context) (openFile, error) {
	body, _, err := s.blobs.Get(ctx, s.key)
	if err != nil {
		return openFile{}, fmt.Errorf("import source %q: %w", s.key, err)
	}
	file := openFile{body: body, records: csv.NewReader(body), key: s.key}
	file.records.TrimLeadingSpace = true
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

	// Indexed by the header EXACTLY as the file spells it, because that is the
	// key ProfileCSV published and the mapping was therefore written against.
	// Trimming here instead would make a header like "Email " unresolvable:
	// its mapped fields would vanish, and a row keyed on it would be skipped.
	index := make(map[string]int, len(file.header))
	for i, name := range file.header {
		index[name] = i
	}

	for {
		record, err := file.records.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		// The reader's own position, not a counter: a quoted field may span
		// several lines, and a disclosure that named the Nth record would send
		// a human to the wrong line of their file.
		line, _ := file.records.FieldPos(0)
		if err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrSourceUnreadable, line, err)
		}
		if err := visit(line, record, index); err != nil {
			return err
		}
	}
}
