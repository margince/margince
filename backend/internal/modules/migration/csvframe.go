// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// How a delimited file is framed. Spreadsheet apps write two things csv.Reader
// does not expect: Excel's "CSV UTF-8" starts with a byte-order mark, and Excel
// in a locale that uses the comma for decimals separates fields with ';'.

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// sniffWindow bounds how much of the header line is read to choose a delimiter.
const sniffWindow = 64 << 10

// newRecordReader is the one reader the profile and the walk share, so the
// columns a mapping is written against are the columns a run reads.
func newRecordReader(r io.Reader) (*csv.Reader, error) {
	buffered := bufio.NewReaderSize(r, sniffWindow)
	if head, err := buffered.Peek(len(byteOrderMark)); err == nil && bytes.Equal(head, byteOrderMark) {
		if _, err := buffered.Discard(len(byteOrderMark)); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
		}
	}
	head, err := buffered.Peek(sniffWindow)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
	}
	reader := csv.NewReader(buffered)
	reader.Comma = delimiterOf(head)
	reader.TrimLeadingSpace = true
	return reader, nil
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
