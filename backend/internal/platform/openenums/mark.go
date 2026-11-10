// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openenums

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarkOpen answers the contract with every closed response-side enum marked,
// and how many it marked. It edits text rather than re-encoding the document,
// so comments and layout stay as written.
//
// A block mapping gets a line after its enum values, at the column of its
// `enum` key. A flow mapping gets the key after the `]` closing its values.
func MarkOpen(raw []byte) ([]byte, int, error) {
	enums, err := Classify(raw)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	after := map[int]string{}
	inline := map[int][]int{}
	marked := 0
	for _, e := range enums {
		if !e.Response || e.Open {
			continue
		}
		key, values := enumKeyAndValues(e.node)
		marked++
		if e.node.Style&yaml.FlowStyle == 0 {
			after[lastLine(values)] = strings.Repeat(" ", key.Column-1) + Marker + ": true"
			continue
		}
		col, err := closingBracket(lines[values.Line-1], values.Column-1)
		if err != nil {
			return nil, 0, fmt.Errorf("line %d: %w", values.Line, err)
		}
		inline[values.Line] = append(inline[values.Line], col)
	}
	for line, cols := range inline {
		sort.Sort(sort.Reverse(sort.IntSlice(cols)))
		text := lines[line-1]
		for _, col := range cols {
			text = text[:col+1] + ", " + Marker + ": true" + text[col+1:]
		}
		lines[line-1] = text
	}
	out := make([]string, 0, len(lines)+len(after))
	for i, line := range lines {
		out = append(out, line)
		if marker, ok := after[i+1]; ok {
			out = append(out, marker)
		}
	}
	return []byte(strings.Join(out, "\n")), marked, nil
}

func enumKeyAndValues(schema *yaml.Node) (key, values *yaml.Node) {
	for i := 0; i+1 < len(schema.Content); i += 2 {
		if schema.Content[i].Value == "enum" {
			return schema.Content[i], schema.Content[i+1]
		}
	}
	return nil, nil
}

// lastLine is the last source line a node spans.
func lastLine(node *yaml.Node) int {
	last := node.Line
	for _, child := range node.Content {
		last = max(last, lastLine(child))
	}
	return last
}

// closingBracket is the byte offset of the `]` closing the flow sequence that
// opens at start. The sequence must close on the same line.
func closingBracket(line string, start int) (int, error) {
	depth, quote := 0, byte(0)
	for i := start; i < len(line); i++ {
		switch ch := line[i]; {
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == '[':
			depth++
		case ch == ']':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("an enum list in a flow mapping does not close on its line: %q; mark it by hand", line)
}
