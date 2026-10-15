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
	path   string
	values map[string]*yaml.Node
}

// readYAML flattens one file to setting keys. A key that names a setting is a
// leaf: a struct or map setting keeps its whole node, and its sub-keys are
// never reported unknown. A missing file is an empty layer.
func readYAML(path string, known func(string) bool, bootstrapRoots map[string]bool) (yamlLayer, []string, error) {
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
	var unknown []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		if bootstrapRoots[name] {
			continue
		}
		walk(name, root.Content[i+1], known, layer.values, &unknown)
	}
	slices.Sort(unknown)
	return layer, unknown, nil
}

// walk skips a null: `key:` with nothing after it, or `key: null`, is how an
// operator leaves a key unwritten without deleting it.
func walk(key string, n *yaml.Node, known func(string) bool, out map[string]*yaml.Node, unknown *[]string) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return
	}
	if known(key) {
		out[key] = n
		return
	}
	if n.Kind != yaml.MappingNode {
		*unknown = append(*unknown, key)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		walk(key+"."+n.Content[i].Value, n.Content[i+1], known, out, unknown)
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
