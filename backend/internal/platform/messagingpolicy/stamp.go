// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// A refresh edits front-matter lines where they stand. Writing the decoded
// keys back as YAML would reorder and reflow a file its maintainers wrote.

// instrumentsKey opens the block list a stamp can edit.
const instrumentsKey = "instruments:"

// stampInstrument writes the digest and its day on the index-th instrument.
func stampInstrument(lines []string, index int, digest, fetched string) ([]string, error) {
	for _, stamp := range [][2]string{{"text-digest", digest}, {"text-fetched", fetched}} {
		start, end, indent, err := instrumentSpan(lines, index)
		if err != nil {
			return nil, err
		}
		lines = setKey(lines, start, end, strings.Repeat(" ", indent)+stamp[0]+":", stamp[1])
	}
	return lines, nil
}

// setKey replaces the line in [start, end) that opens with prefix, or adds
// one at end.
func setKey(lines []string, start, end int, prefix, value string) []string {
	for i := start; i < end; i++ {
		if strings.HasPrefix(lines[i], prefix) {
			lines[i] = prefix + " " + value
			return lines
		}
	}
	return slices.Insert(lines, end, prefix+" "+value)
}

// setTopKey writes a key of the layer itself: over its own line, or else
// below created, where the other provenance keys sit.
func setTopKey(lines []string, key, value string) []string {
	at := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "created:") {
			at = i + 1
		}
	}
	if own := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, key+":") }); own >= 0 {
		return setKey(lines, own, own+1, key+":", value)
	}
	return setKey(lines, at, at, key+":", value)
}

// instrumentSpan answers the lines of the index-th instrument and the indent
// its keys take. It reads a block list, one "- " item per instrument.
func instrumentSpan(lines []string, index int) (start, end, indent int, err error) {
	head := slices.Index(lines, instrumentsKey)
	if head < 0 {
		return 0, 0, 0, errors.New("a stamp needs the instruments as a block list under a line of their own")
	}
	starts, listEnd, itemIndent := blockItems(lines, head+1)
	if index >= len(starts) {
		return 0, 0, 0, fmt.Errorf("the instruments list has %d block items, and the stamp is for item %d", len(starts), index+1)
	}
	end = listEnd
	if index+1 < len(starts) {
		end = starts[index+1]
	}
	for end > starts[index]+1 && isBlankOrComment(lines[end-1]) {
		end--
	}
	return starts[index], end, itemIndent + len("- "), nil
}

// blockItems answers where each item of the block list opening at from
// starts, where the list ends, and the indent of its "- " markers.
func blockItems(lines []string, from int) (starts []int, end, itemIndent int) {
	itemIndent = -1
	for i := from; i < len(lines); i++ {
		if isBlankOrComment(lines[i]) {
			continue
		}
		text := strings.TrimLeft(lines[i], " ")
		depth := len(lines[i]) - len(text)
		isItem := strings.HasPrefix(text, "- ")
		if itemIndent < 0 && isItem {
			itemIndent = depth
		}
		if itemIndent < 0 || depth < itemIndent || (depth == itemIndent && !isItem) {
			return starts, i, itemIndent
		}
		if depth == itemIndent {
			starts = append(starts, i)
		}
	}
	return starts, len(lines), itemIndent
}

func isBlankOrComment(line string) bool {
	text := strings.TrimLeft(line, " ")
	return text == "" || strings.HasPrefix(text, "#")
}
