// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"
)

// kind is one entry of api/worklist-kinds.yaml. The booleans are pointers so a
// field left out is told apart from false and refused.
type kind struct {
	Category    string   `yaml:"category"`
	Destination string   `yaml:"destination"`
	Owner       []string `yaml:"owner"`
	Personal    *bool    `yaml:"personal"`
	DoneInPlace *bool    `yaml:"done_in_place"`
}

// catalog maps a kind's name to its declaration.
type catalog map[string]kind

var (
	categories   = []string{"customer_waiting", "leads", "deals_at_risk", "meetings", "tasks", "decisions", "relationships", "system"}
	destinations = []string{"today", "review", "system_health", "none"}
	owners       = []string{"unstated", "named", "nobody", "the_reader", "from_the_deal", "unreadable"}
	kindName     = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
)

// parseCatalog decodes and checks a kinds document. It refuses an unknown or
// missing field, a value outside its set and a second YAML document.
func parseCatalog(raw []byte) (catalog, error) {
	var parsed struct {
		Kinds catalog `yaml:"kinds"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("worklist kinds: %w", err)
	}
	var second yaml.Node
	if err := decoder.Decode(&second); !errors.Is(err, io.EOF) {
		return nil, errors.New("worklist kinds: keep every kind in one YAML document; a second one would reach neither table")
	}
	if len(parsed.Kinds) == 0 {
		return nil, errors.New("worklist kinds: the document declares no kinds")
	}
	for _, name := range parsed.Kinds.sortedNames() {
		if err := parsed.Kinds[name].check(name); err != nil {
			return nil, fmt.Errorf("worklist kind %q: %w", name, err)
		}
	}
	return parsed.Kinds, nil
}

func (k kind) check(name string) error {
	if !kindName.MatchString(name) {
		return errors.New("a kind's name is lower-case letters and underscores, the WorklistItem.source value")
	}
	if err := oneOf("category", k.Category, categories); err != nil {
		return err
	}
	if err := oneOf("destination", k.Destination, destinations); err != nil {
		return err
	}
	if len(k.Owner) == 0 {
		return errors.New("owner names at least one answer")
	}
	for i, answer := range k.Owner {
		if err := oneOf("owner", answer, owners); err != nil {
			return err
		}
		if slices.Contains(k.Owner[:i], answer) {
			return fmt.Errorf("owner names %q twice", answer)
		}
	}
	if k.Personal == nil {
		return errors.New("personal is missing: state true or false")
	}
	if k.DoneInPlace == nil {
		return errors.New("done_in_place is missing: state true or false")
	}
	return nil
}

func oneOf(field, value string, allowed []string) error {
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s %q is not one of %v", field, value, allowed)
	}
	return nil
}

// sortedNames is the one order both emitted tables walk, so a run is byte-stable.
func (c catalog) sortedNames() []string {
	names := make([]string, 0, len(c))
	for name := range c {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
