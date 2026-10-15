// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

var errLiteralSecret = errors.New("a secret in YAML must be a ${file:...} or ${env:...} reference")

type refReaders struct {
	readFile func(string) ([]byte, error)
	getenv   func(string) (string, bool)
}

// resolve takes the first layer that holds a candidate. A stored setting with
// none stays unset, because the database decides it later.
func resolve(s Any, layers []layer, readFile func(string) ([]byte, error), getenv func(string) (string, bool)) (resolved, error) {
	refs := refReaders{readFile: readFile, getenv: getenv}
	for _, l := range layers {
		if text, ok := l.text[s.Key()]; ok {
			return resolveText(s, l, text, refs)
		}
		if node, ok := l.nodes[s.Key()]; ok {
			return resolveNode(s, l, node, refs)
		}
	}
	if s.Stored() {
		return resolved{}, nil
	}
	if v, ok := s.defaultValue(); ok {
		return resolved{value: v, origin: Origin{Source: SourceDefault}, set: true}, nil
	}
	return resolved{}, fmt.Errorf("%s: required", s.Env())
}

func resolveText(s Any, l layer, text string, refs refReaders) (resolved, error) {
	origin := Origin{Source: l.source, Name: l.names[s.Key()], File: l.file}
	fault := func(err error) error { return fmt.Errorf("%s (%s): %w", origin.Name, l.source, err) }
	if s.Secret() {
		value, file, _, err := resolveRef(text, refs.readFile, refs.getenv)
		if err != nil {
			return resolved{}, fault(err)
		}
		if file != "" {
			origin.File = file
		}
		text = value
	}
	v, err := s.parseText(text)
	if err == nil {
		err = s.check(v)
	}
	if err != nil {
		return resolved{}, fault(err)
	}
	return resolved{value: v, origin: origin, set: true}, nil
}

// resolveNode refuses a literal secret in YAML: a file that is read, copied
// and committed is the wrong place for a credential.
func resolveNode(s Any, l layer, node *yaml.Node, refs refReaders) (resolved, error) {
	origin := Origin{Source: l.source, Name: s.Key(), File: l.file}
	fault := func(err error) error { return fmt.Errorf("%s in %s (%s): %w", s.Key(), l.file, l.source, err) }
	if !s.Secret() {
		v, err := s.decodeYAML(node)
		if err == nil {
			err = s.check(v)
		}
		if err != nil {
			return resolved{}, fault(err)
		}
		return resolved{value: v, origin: origin, set: true}, nil
	}
	if node.Kind != yaml.ScalarNode {
		return resolved{}, fault(errLiteralSecret)
	}
	value, file, isRef, err := resolveRef(node.Value, refs.readFile, refs.getenv)
	switch {
	case err != nil:
		return resolved{}, fault(err)
	case !isRef:
		return resolved{}, fault(errLiteralSecret)
	case file != "":
		origin.File = file
	}
	v, err := s.parseText(value)
	if err == nil {
		err = s.check(v)
	}
	if err != nil {
		return resolved{}, fault(err)
	}
	return resolved{value: v, origin: origin, set: true}, nil
}

// undeclared names the variables no program reads, from the environment and
// the .env file alike. Values never leave this function.
func undeclared(known func(string) bool, layers []layer) []string {
	seen := map[string]bool{}
	for _, l := range layers {
		for name, value := range l.vars {
			if reportable(name, value) && !reserved[name] && !known(name) {
				seen[name] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}
