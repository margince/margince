// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type yamlLayer struct {
	path     string
	values   map[string]*yaml.Node
	repeated []error
	envRefs  []string
}

// unknownKey is a key no program reads, and the line that writes it.
type unknownKey struct {
	key  string
	line int
}

// readYAML flattens one file to setting keys. A key that names a setting is a
// leaf: a struct or map setting keeps its whole node, and its sub-keys are
// never reported unknown. A missing file is an empty layer.
func readYAML(path string, names known) (yamlLayer, []unknownKey, error) {
	layer := yamlLayer{path: path, values: map[string]*yaml.Node{}}
	raw, err := os.ReadFile(path) // #nosec G304 -- the operator's own --config / --config-file path
	if errors.Is(err, os.ErrNotExist) {
		return layer, nil, nil
	}
	if err != nil {
		return layer, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return layer, nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return layer, nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return layer, nil, fmt.Errorf("%s: the top level must be a mapping", path)
	}
	layer.repeated = repeatedKeys(path, "", root)
	layer.envRefs = envRefsIn(root)
	var unknown []unknownKey
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		if names.bootstrapRoots[name] {
			continue
		}
		walk(name, root.Content[i], root.Content[i+1], names, layer.values, &unknown)
	}
	slices.SortFunc(unknown, func(a, b unknownKey) int { return strings.Compare(a.key, b.key) })
	return layer, unknown, nil
}

// repeatedKeys finds a key written twice in one mapping, anywhere in the file,
// bootstrap sections included. The parser keeps the last one silently, so an
// operator who pasted a block below an older copy cannot tell which one wins.
func repeatedKeys(path, prefix string, n *yaml.Node) []error {
	var faults []error
	switch n.Kind {
	case yaml.MappingNode:
		firstLine := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			key := keyNode.Value
			if prefix != "" {
				key = prefix + "." + key
			}
			if line, seen := firstLine[keyNode.Value]; seen {
				faults = append(faults, fmt.Errorf("%s: %s appears on lines %d and %d; keep one", path, key, line, keyNode.Line))
			} else {
				firstLine[keyNode.Value] = keyNode.Line
			}
			faults = append(faults, repeatedKeys(path, key, n.Content[i+1])...)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			faults = append(faults, repeatedKeys(path, prefix, item)...)
		}
	}
	return faults
}

// envRefsIn names every variable a ${env:NAME} value in the document reads, in
// any section. A reference under a bootstrap root or another program's setting
// still consumes its variable.
func envRefsIn(n *yaml.Node) []string {
	if n.Kind == yaml.ScalarNode {
		if name, ok := envRefName(n.Value); ok {
			return []string{name}
		}
		return nil
	}
	var names []string
	for _, child := range n.Content {
		names = append(names, envRefsIn(child)...)
	}
	return names
}

// walk skips a null: `key:` with nothing after it, or `key: null`, is how an
// operator leaves a key unwritten without deleting it. A mapping under a name
// that is no section is one unknown key, not one per leaf below it.
func walk(key string, keyNode, n *yaml.Node, names known, out map[string]*yaml.Node, unknown *[]unknownKey) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return
	}
	if names.keys[key] {
		out[key] = n
		return
	}
	if n.Kind != yaml.MappingNode || !names.sections[key] {
		*unknown = append(*unknown, unknownKey{key: key, line: keyNode.Line})
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		walk(key+"."+n.Content[i].Value, n.Content[i], n.Content[i+1], names, out, unknown)
	}
}

var overlayPostures = []string{"dev", "test", "prod", "production", "staging"}

// overlayBeside finds the old per-posture files (margince.dev.yaml) so the
// loader can refuse them. Only the postures runtimeenv knows match, so
// templates such as margince.example.yaml never trip it.
func overlayBeside(base string) []string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	var found []string
	for _, p := range overlayPostures {
		candidate := stem + "." + p + ext
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		}
	}
	return found
}
