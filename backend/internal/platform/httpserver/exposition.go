// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"fmt"
	"io"
)

// exposition is the one thing every /readyz and /metrics section writes
// through, and the reason none of them handles a write error.
//
// Two postures used to sit in this file with nothing saying which was which.
// The job section returned its write error and the handler stopped, because a
// truncated job section PARSES as a smaller fleet and an alert acts on it.
// Every other section discarded its errors, on the equally sound reasoning that
// a refused write means the scraper hung up and there is nothing left to report
// to. The distinction was invisible, so a section added later inherited
// whichever its author copied — the runtime section inherited the silent one,
// and nothing said it should not have.
//
// There is one posture now, and it belongs to the writer rather than to each
// section: the FIRST refused write is remembered and every write after it is a
// no-op. A section cannot half-write, nothing downstream is pushed into a
// socket that is gone, and the handler asks once, at the end, whether the
// exposition it just assembled actually left.
//
// It is an io.Writer, which is what makes it reach the sections this package
// does not own — the injected `extra` families and the job section — without
// changing what they are handed. Their own discarded errors become sound here:
// the first failure is recorded whether or not the caller looked.
type exposition struct {
	w   io.Writer
	err error
}

// Write records the first failure and refuses everything after it.
//
// The short-circuit returns the REMEMBERED error, not nil: a writer that
// claimed success after failing would let a section that does check its errors
// carry on assembling into nothing.
func (e *exposition) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}

// gone reports that a write was already refused, so this scrape is not going to
// be delivered. The pure-rendering sections need no such check — printf is
// already a no-op by then — but a section that MEASURES does: a database read
// for a socket that is not there is work nobody will see the result of.
func (e *exposition) gone() bool { return e.err != nil }

// printf is how the sections in THIS file write. The error is not dropped — it
// is held on the exposition, which is the only place any section's write error
// was ever going to be actionable.
func (e *exposition) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}
