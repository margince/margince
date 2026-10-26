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

var (
	errLiteralSecret = errors.New("a secret in YAML must be a ${file:...} or ${env:...} reference")
	errInvalidSecret = errors.New("is not a valid value")
)

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
	// The type decides whether unset is allowed: String takes empty text, so an
	// optional secret needs no default, which a secret may not carry.
	if _, err := parseChecked(s, ""); err == nil {
		return resolved{origin: Origin{Source: SourceUnset}}, nil
	}
	return resolved{}, fmt.Errorf("%s: required", s.Env())
}

func resolveText(s Any, l layer, text string, refs refReaders) (resolved, error) {
	origin := Origin{Source: l.source, Name: l.names[s.Key()], File: l.file}
	fault := func(err error) error { return fmt.Errorf("%s (%s): %w", origin.Name, l.source, err) }
	if s.Secret() && declaredNone(text) {
		return resolveNone(s, origin, fault)
	}
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
	v, err := parseChecked(s, text)
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
	if declaredNone(node.Value) {
		return resolveNone(s, origin, fault)
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
	v, err := parseChecked(s, value)
	if err != nil {
		return resolved{}, fault(err)
	}
	return resolved{value: v, origin: origin, set: true}, nil
}

// resolveNone is a secret declared absent: the empty value, marked so a
// reader can tell it from a secret nobody declared.
func resolveNone(s Any, origin Origin, fault func(error) error) (resolved, error) {
	if !s.allowNone() {
		return resolved{}, fault(errNoneRefused)
	}
	v, err := s.parseText("")
	if err != nil {
		return resolved{}, fault(err)
	}
	origin.None = true
	return resolved{value: v, origin: origin, set: true}, nil
}

// parseChecked cannot leak a secret: a Type's parse error never quotes its
// input, and checkTyped drops the reason a secret's Validate gives.
//
//craft:ignore naked-any the registry holds settings of many types behind one erased view
func parseChecked(s Any, text string) (any, error) {
	v, err := s.parseText(text)
	if err != nil {
		return nil, err
	}
	return v, s.check(v)
}

// undeclared names the variables no program reads, from the environment and
// the .env file alike, each with the name it likely meant. A variable that a
// ${env:NAME} reference reads is consumed, so it is not reported. Values never
// leave this function.
func undeclared(names known, layers []layer) []string {
	referenced := envReferenced(layers)
	seen := map[string]bool{}
	for _, l := range layers {
		for name, value := range l.vars {
			if !reportable(name, value) || reserved[name] || names.envs[name] || referenced[name] {
				continue
			}
			if near := names.nearEnv(name); near != "" {
				name += " (did you mean " + near + "?)"
			}
			seen[name] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// readElsewhere names the settings the sources set that only a sibling program
// reads. This program never parses their values, so its check cannot vouch for
// them, and says so instead of calling the file valid.
func readElsewhere(reg *Registry, names known, layers []layer) []string {
	seen := map[string]bool{}
	for _, l := range layers {
		where := string(l.source)
		if l.file != "" {
			where += " " + l.file
		}
		for key := range l.nodes {
			if _, own := reg.byKey[key]; !own {
				seen[key+" ("+where+")"] = true
			}
		}
		for name, value := range l.vars {
			if _, own := reg.byEnv[name]; value != "" && !own && names.envs[name] {
				seen[name+" ("+where+")"] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// envReferenced collects the variables ${env:NAME} references read, from a
// YAML file, a flag, the environment or the .env file.
func envReferenced(layers []layer) map[string]bool {
	out := map[string]bool{}
	for _, l := range layers {
		for _, name := range l.envRefs {
			out[name] = true
		}
		for _, values := range []map[string]string{l.text, l.vars} {
			for _, value := range values {
				if name, ok := envRefName(value); ok {
					out[name] = true
				}
			}
		}
	}
	return out
}
