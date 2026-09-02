// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The one JSONL file this package knows how to write: create it under a
// caller-named directory, announce its absolute path on stdout, encode lines to
// it under a lock, close it.
//
// Two side-channels the cert lane writes are this plus their own policy — the
// payload trace (trace.go) and the resume journal (resume.go). They differ in
// what they write and in whether they truncate or append; they do not differ in
// how a line reaches the disk, and a second copy of that would be two answers to
// "did the announce fail, and did we still leave a file open" — the sort of
// question one of the copies eventually answers differently.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// jsonlSink is an open JSONL file plus the lock serializing writes to it. A nil
// *jsonlSink is the disabled state: encode and close no-op on it, so an owner
// whose feature is switched off threads one value instead of branching.
//
// The encoder writes STRAIGHT to the file, with nothing buffered in front of it.
// The resume journal depends on that outright — a journal flushed on close would
// be lost to exactly the kill it exists to survive, and would fail silently,
// looking like one with nothing to replay — so a buffer added here would break a
// caller that has no way to ask for its absence.
type jsonlSink struct {
	mu  sync.Mutex
	w   io.WriteCloser
	enc *json.Encoder
	// broken is the first write failure, after which the sink refuses rather
	// than appending behind a line a reader will stop at. See encodeLine.
	broken error
	Path   string // absolute, printed to stdout when the sink opens
}

// openJSONLSink opens dir/name with flags and returns the sink writing to it,
// after printing the absolute path under the announce label so an operator can
// open the file while the run is still going.
//
// Mode 0o600 for both callers: the files hold post-stripper model payloads, and
// nothing but the operator who ran the lane has business reading them.
func openJSONLSink(dir, name, announce string, flags int) (*jsonlSink, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("aicert: %s dir %s: %w", announce, dir, err)
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, flags, 0o600) // #nosec G304 -- an operator-named dir (MARGINCE_AICERT_TRACE / _RESUME) plus a fixed filename; a dev lane, no request input
	if err != nil {
		return nil, fmt.Errorf("aicert: open %s %s: %w", announce, path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		// A resolvable path failing to absolutize is a real filesystem fault,
		// not a caller input — fail the run rather than print a path nothing
		// can be trusted to find again.
		//craft:ignore swallowed-errors close-then-report on the error path
		_ = f.Close()
		return nil, fmt.Errorf("aicert: absolute %s path for %s: %w", announce, path, err)
	}
	if _, err := fmt.Fprintf(os.Stdout, "aicert: %s → %s\n", announce, abs); err != nil {
		//craft:ignore swallowed-errors close-then-report on the error path
		_ = f.Close()
		return nil, fmt.Errorf("aicert: announce %s path %s: %w", announce, abs, err)
	}
	return &jsonlSink{w: f, enc: json.NewEncoder(f), Path: abs}, nil
}

// encodeLine appends one line. Nil-safe on s, so an owner whose feature is off
// calls it unconditionally.
//
// The type parameter names the two line shapes this package writes rather than
// taking any: a sink that accepted anything would let a caller write a shape
// nothing here can read back, and the resume journal is read back.
func encodeLine[T tracedCall | journaledRun](s *jsonlSink, line T) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A failed write may already have put a partial line on disk, and a reader
	// stops at the first line it cannot decode. Appending past one would leave
	// every LATER run stranded behind it — the file would keep growing and keep
	// replaying nothing, which reads exactly like a journal that never had
	// anything to replay. One failure closes the sink instead.
	if s.broken != nil {
		return fmt.Errorf("aicert: %s stopped at an earlier failed write: %w", s.Path, s.broken)
	}
	if err := s.enc.Encode(line); err != nil {
		s.broken = err
		return fmt.Errorf("aicert: write %s: %w", s.Path, err)
	}
	return nil
}

// close closes the underlying file. Nil-safe, so an owner can defer it
// unconditionally.
func (s *jsonlSink) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Close()
}
