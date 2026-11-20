// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// unfoldVCardLines joins continuation lines before anything parses them. A long
// value is split across lines, and each continuation starts with a space or tab.
func unfoldVCardLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	// A line may be as long as the file may be. An embedded photo on one unfolded
	// line imports as it does folded. A file past the bound is refused below in
	// the product's words, not the scanner's.
	scanner.Buffer(make([]byte, 0, 64*1024), vcardMaxBytes+2)
	var lines []string
	// The line under construction grows in one buffer. Rebuilding it per
	// continuation copies the whole line each time, which is quadratic work.
	var folding []byte
	var open, quotedPrintable, sawColon bool
	flush := func() {
		if open {
			lines = append(lines, string(folding))
			folding = folding[:0]
			open = false
		}
	}
	read := 0
	for scanner.Scan() {
		read += len(scanner.Bytes()) + 1
		if read > vcardMaxBytes {
			// The limited reader hands back a prefix. Parsing it would import
			// part of an address book without a word.
			return nil, fmt.Errorf("the vCard file exceeds %d bytes", vcardMaxBytes)
		}
		if len(lines) > vcardMaxLines {
			return nil, fmt.Errorf("the vCard file has more than %d lines", vcardMaxLines)
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		if open && quotedPrintable && folding[len(folding)-1] == '=' {
			// A soft line break: the "=" ends the physical line and the value
			// goes on in the next one, with no leading space.
			folding = append(folding[:len(folding)-1], line...)
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && open {
			folding = append(folding, line[1:]...)
			if !sawColon && strings.Contains(line, ":") {
				sawColon, quotedPrintable = true, quotedPrintableLine(string(folding))
			}
			continue
		}
		flush()
		folding = append(folding, line...)
		// The parameters sit on the property's first physical line, so the
		// encoding is read once per property, not once per continuation.
		sawColon = strings.Contains(line, ":")
		quotedPrintable = quotedPrintableLine(line)
		open = true
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the vCard file: %w", err)
	}
	return lines, nil
}

// quotedPrintableLine says whether an unfolded line's parameters declare
// quoted-printable, the only encoding whose trailing "=" is a soft line break.
func quotedPrintableLine(line string) bool {
	colon := indexUnquoted(line, ':')
	return colon > 0 && strings.Contains(strings.ToUpper(line[:colon]), "QUOTED-PRINTABLE")
}
