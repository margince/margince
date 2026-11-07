// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package doctext reads a bounded text excerpt from an office document or a
// text file, and names which files count as documents at all.
//
// It is pdftext's neighbour for the formats that are not PDF. Untrusted bytes go
// in and a capped string comes out; nothing is written or executed. An xlsx or
// docx is a zip of XML. The hazards are the archive's (a bomb, ten thousand
// entries) and the parser's (one token the size of the file). Every read is
// bounded by a byte budget, and the header's declared sizes are never trusted.
//
// The format is told from the bytes, not from the stored content type. Capture
// stores the sniffed type, and a sniffer reports every xlsx as application/zip.
package doctext

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// ErrUnsupported is a file this package does not read: a legacy office file,
// a zip of something else, an image.
var ErrUnsupported = errors.New("doctext: this file is not a format that can be read as text")

// ErrUnreadable is a file of a supported format that could not be parsed:
// truncated, malformed, or larger than the archive bounds allow.
var ErrUnreadable = errors.New("doctext: this file could not be read")

// ErrNoText is a file that parsed and states nothing: an empty sheet, a
// document of images.
var ErrNoText = errors.New("doctext: this file carries no text")

// Archive bounds. entryBytes caps one inflated entry and totalBytes the whole
// file. maxEntries caps the central directory: a workbook of a few sheets has
// a few dozen entries.
const (
	maxEntries = 512
	entryBytes = 4 << 20
	totalBytes = 12 << 20
	// maxSharedStrings bounds the dictionary a workbook may make this package
	// hold; entryBytes already bounds its bytes, this bounds its slice.
	maxSharedStrings = 200_000
	// cancelEvery is how many XML tokens pass between context checks.
	cancelEvery = 4096
)

// Excerpt is what a file states, cut to the caller's budget.
type Excerpt struct {
	Text string
	// Truncated says the file states more than Text holds, so a reader of the
	// excerpt is not told it saw the whole document.
	Truncated bool
}

// Read returns at most maxChars runes of the text the file states.
func Read(ctx context.Context, raw []byte, maxChars int) (Excerpt, error) {
	if maxChars <= 0 {
		return Excerpt{}, fmt.Errorf("doctext: maxChars must be positive, got %d", maxChars)
	}
	out := &excerptWriter{limit: maxChars}
	var err error
	switch {
	case bytes.HasPrefix(raw, []byte("PK\x03\x04")):
		err = readOffice(ctx, raw, out)
	case strings.HasPrefix(http.DetectContentType(raw), "text/") && utf8.Valid(raw):
		err = out.write(string(raw))
	default:
		return Excerpt{}, ErrUnsupported
	}
	if err != nil && !errors.Is(err, errFull) {
		return Excerpt{}, err
	}
	text := strings.TrimSpace(out.text.String())
	if text == "" {
		return Excerpt{}, ErrNoText
	}
	return Excerpt{Text: text, Truncated: out.truncated}, nil
}

// readOffice opens the archive and reads whichever office format it holds.
func readOffice(ctx context.Context, raw []byte, out *excerptWriter) error {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if len(archive.File) > maxEntries {
		return fmt.Errorf("%w: the archive has %d entries", ErrUnreadable, len(archive.File))
	}
	parts := &archiveParts{files: map[string]*zip.File{}, left: totalBytes}
	for _, f := range archive.File {
		parts.files[f.Name] = f
	}
	switch {
	case parts.has("xl/workbook.xml"):
		return readWorkbook(ctx, parts, out)
	case parts.has("word/document.xml"):
		return readDocument(ctx, parts, out)
	}
	return ErrUnsupported
}

// errFull stops a walk once the excerpt holds all the caller asked for.
var errFull = errors.New("doctext: the excerpt is full")

// excerptWriter collects text up to a rune budget and remembers whether
// anything was left out.
type excerptWriter struct {
	text      strings.Builder
	held      int
	limit     int
	truncated bool
}

// write appends s, or as much of it as the budget allows, and answers errFull
// once nothing more fits.
func (w *excerptWriter) write(s string) error {
	for _, r := range s {
		if w.held == w.limit {
			w.truncated = true
			return errFull
		}
		w.text.WriteRune(r)
		w.held++
	}
	return nil
}

// archiveParts reads entries of one archive against one shared byte budget.
type archiveParts struct {
	files map[string]*zip.File
	left  int64
}

func (p *archiveParts) has(name string) bool {
	_, ok := p.files[name]
	return ok
}

// open returns a reader over one inflated entry that fails with errBudget
// past entryBytes or past what the archive as a whole has left.
func (p *archiveParts) open(name string) (io.ReadCloser, error) {
	f, ok := p.files[name]
	if !ok {
		return nil, fmt.Errorf("%w: missing part %s", ErrUnreadable, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return &budgetReader{rc: rc, entry: entryBytes, parts: p}, nil
}

// errBudget is an archive that inflates past its bounds.
var errBudget = fmt.Errorf("%w: the archive inflates past its bounds", ErrUnreadable)

// budgetReader charges every inflated byte to the entry and to the archive.
type budgetReader struct {
	rc    io.ReadCloser
	entry int64
	parts *archiveParts
}

func (b *budgetReader) Read(p []byte) (int, error) {
	allowed := min(b.entry, b.parts.left)
	if allowed <= 0 {
		return 0, errBudget
	}
	if int64(len(p)) > allowed {
		p = p[:allowed]
	}
	n, err := b.rc.Read(p)
	b.entry -= int64(n)
	b.parts.left -= int64(n)
	return n, err
}

func (b *budgetReader) Close() error { return b.rc.Close() }
