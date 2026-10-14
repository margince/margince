// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// How a delimited file is framed. Spreadsheet apps write two things csv.Reader
// does not expect. Excel's "CSV UTF-8" starts with a byte-order mark. Excel in
// a locale with a decimal comma separates fields with ';'.

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
)

var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// sniffWindow bounds how much of the header line is read to choose a delimiter.
const sniffWindow = 64 << 10

// recordReader removes the formula guard an export put on a cell, so an
// unedited export imports as the values it was written from.
type recordReader struct {
	*csv.Reader
}

func (r recordReader) Read() ([]string, error) {
	record, err := r.Reader.Read()
	for i, cell := range record {
		record[i] = formulacell.Unguard(cell)
	}
	return record, err
}

// newRecordReader is the reader both the profile and the walk use, so the
// columns a mapping is written against are the columns a run reads.
func newRecordReader(r io.Reader) (recordReader, error) {
	buffered := bufio.NewReaderSize(r, sniffWindow)
	if head, err := buffered.Peek(len(byteOrderMark)); err == nil && bytes.Equal(head, byteOrderMark) {
		if _, err := buffered.Discard(len(byteOrderMark)); err != nil {
			return recordReader{}, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
		}
	}
	head, err := buffered.Peek(sniffWindow)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return recordReader{}, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
	}
	reader := csv.NewReader(buffered)
	reader.Comma = delimiterOf(head)
	reader.TrimLeadingSpace = true
	return recordReader{reader}, nil
}

// delimiterOf counts candidate delimiters outside quotes on the first line and
// picks the most frequent; a tie, or none at all, keeps the comma.
func delimiterOf(head []byte) rune {
	counts := map[rune]int{}
	quoted := false
	for _, b := range head {
		switch {
		case b == '"':
			quoted = !quoted
		case !quoted && (b == '\n' || b == '\r'):
			return mostFrequent(counts)
		case !quoted && (b == ',' || b == ';' || b == '\t'):
			counts[rune(b)]++
		}
	}
	return mostFrequent(counts)
}

func mostFrequent(counts map[rune]int) rune {
	best := ','
	for _, d := range []rune{';', '\t'} {
		if counts[d] > counts[best] {
			best = d
		}
	}
	return best
}
