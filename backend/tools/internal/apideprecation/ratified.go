// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apideprecation

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// RatifiedRemovals maps an operation key ("POST /v1/path") to the reason a
// maintainer gave for removing it without a published notice.
type RatifiedRemovals map[string]string

// ParseRatifiedRemovals reads one "METHOD /path reason" line per operation.
// A line without a reason is refused, so every exception says why it exists.
func ParseRatifiedRemovals(r io.Reader) (RatifiedRemovals, error) {
	removals := RatifiedRemovals{}
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) < 3 {
			return nil, fmt.Errorf("ratified removal line %d: want METHOD /path reason", line)
		}
		key := strings.ToUpper(fields[0]) + " " + fields[1]
		if _, dup := removals[key]; dup {
			return nil, fmt.Errorf("ratified removal line %d: %s is listed twice", line, key)
		}
		removals[key] = strings.Join(fields[2:], " ")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ratified removals: %w", err)
	}
	return removals, nil
}
