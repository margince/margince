// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// writeJSON writes the YAML contract as JSON node by node. Decoding into a map
// would sort the keys, and a reader expects `openapi` and `info` first.
func writeJSON(out *bytes.Buffer, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		return writeJSON(out, n.Content[0])
	case yaml.MappingNode:
		return writeObject(out, n)
	case yaml.SequenceNode:
		out.WriteByte('[')
		for i, item := range n.Content {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
		return nil
	case yaml.AliasNode:
		return writeJSON(out, n.Alias)
	case yaml.ScalarNode:
		return writeScalar(out, n)
	}
	return fmt.Errorf("agentbundle: line %d holds a YAML node of kind %d, which JSON has no form for", n.Line, n.Kind)
}

func writeObject(out *bytes.Buffer, n *yaml.Node) error {
	out.WriteByte('{')
	for i := 0; i+1 < len(n.Content); i += 2 {
		if i > 0 {
			out.WriteByte(',')
		}
		if err := writeString(out, n.Content[i].Value); err != nil {
			return err
		}
		out.WriteByte(':')
		if err := writeJSON(out, n.Content[i+1]); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}

// writeScalar keeps the type YAML resolved: an unquoted 200 stays a number and
// a quoted '200' a string. A tag JSON lacks, such as a timestamp, is its text.
func writeScalar(out *bytes.Buffer, n *yaml.Node) error {
	switch n.ShortTag() {
	case "!!null":
		out.WriteString("null")
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return fmt.Errorf("agentbundle: line %d: %w", n.Line, err)
		}
		out.WriteString(strconv.FormatBool(b))
	case "!!int":
		var i int64
		if err := n.Decode(&i); err != nil {
			return fmt.Errorf("agentbundle: line %d: %w", n.Line, err)
		}
		out.WriteString(strconv.FormatInt(i, 10))
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil {
			return fmt.Errorf("agentbundle: line %d: %w", n.Line, err)
		}
		number, err := json.Marshal(f)
		if err != nil {
			return fmt.Errorf("agentbundle: line %d holds %q, which JSON cannot carry: %w", n.Line, n.Value, err)
		}
		out.Write(number)
	default:
		return writeString(out, n.Value)
	}
	return nil
}

func writeString(out *bytes.Buffer, s string) error {
	quoted, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("agentbundle: quoting a string: %w", err)
	}
	out.Write(quoted)
	return nil
}
