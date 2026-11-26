// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// fileChange is what a diff did to one file, in the line numbers of the new
// side. A deletion has no new line of its own, so it is kept as the line it
// follows.
type fileChange struct {
	changed      map[int]bool
	deletedAfter []int
}

// touches reports whether the change reaches the lines first to last. A
// deletion counts only when both of its neighbours are in the range, so lines
// removed between two declarations touch neither.
func (c *fileChange) touches(first, last int) bool {
	for line := first; line <= last; line++ {
		if c.changed[line] {
			return true
		}
	}
	for _, after := range c.deletedAfter {
		if after >= first && after < last {
			return true
		}
	}
	return false
}

// parseDiff reads `git diff -U0` output into the changes of each file the new
// side still has, keyed by its repository path.
func parseDiff(r io.Reader) (map[string]*fileChange, error) {
	files := map[string]*fileChange{}
	var current *fileChange
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = nil
			if path, ok := strings.CutPrefix(line, "+++ b/"); ok {
				if files[path] == nil {
					files[path] = &fileChange{changed: map[int]bool{}}
				}
				current = files[path]
			}
		case strings.HasPrefix(line, "@@ ") && current != nil:
			start, length, err := newSide(line)
			if err != nil {
				return nil, err
			}
			if length == 0 {
				current.deletedAfter = append(current.deletedAfter, start)
			}
			for n := start; n < start+length; n++ {
				current.changed[n] = true
			}
		}
	}
	return files, scanner.Err()
}

// newSide reads the `+start,length` half of a hunk header. A missing length
// means one line.
func newSide(header string) (start, length int, err error) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, fmt.Errorf("hunk header %q has no new side", header)
	}
	startText, lengthText, hasLength := strings.Cut(fields[2][1:], ",")
	if start, err = strconv.Atoi(startText); err != nil {
		return 0, 0, fmt.Errorf("hunk header %q: %w", header, err)
	}
	if !hasLength {
		return start, 1, nil
	}
	if length, err = strconv.Atoi(lengthText); err != nil {
		return 0, 0, fmt.Errorf("hunk header %q: %w", header, err)
	}
	return start, length, nil
}
