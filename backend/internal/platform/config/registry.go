// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Any is the type-erased view the registry, loader, docs and seam use. Its
// unexported methods keep decoding with the loader.
type Any interface {
	Key() string
	Env() string
	Flag() string
	TypeName() string
	Doc() string
	Secret() bool
	Stored() bool
	KeptByDataReset() bool
	Roles() []Role
	DefaultText() (string, bool)
	Shape() (Shape, error)
	parseText(string) (any, error)
	decodeYAML(n *yaml.Node) (any, error)
	check(any) error
	allowNone() bool
	format(any) string
	defaultValue() (any, bool)
}

var (
	keyForm  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	envForm  = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	flagForm = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

const (
	envPosture    = "MARGINCE_ENV"
	envConfig     = "MARGINCE_CONFIG"
	envConfigFile = "MARGINCE_CONFIG_FILE"
	envEnvFile    = "MARGINCE_ENV_FILE"
	envTrace      = "MARGINCE_CONFIG_TRACE"
)

var reserved = reservedNames()

// Registry indexes the settings by key, env name and flag.
type Registry struct {
	byKey, byEnv, byFlag map[string]Any
	ordered              []Any
}

// NewRegistry joins the groups and reports every fault at once.
func NewRegistry(groups ...[]Any) (*Registry, error) {
	r := &Registry{byKey: map[string]Any{}, byEnv: map[string]Any{}, byFlag: map[string]Any{}}
	var faults []error
	for _, group := range groups {
		for _, s := range group {
			if err := r.add(s); err != nil {
				faults = append(faults, err)
			}
		}
	}
	if err := errors.Join(faults...); err != nil {
		return nil, err
	}
	slices.SortFunc(r.ordered, func(a, b Any) int { return strings.Compare(a.Key(), b.Key()) })
	return r, nil
}

func (r *Registry) add(s Any) error {
	if err := wellFormed(s); err != nil {
		return err
	}
	for _, n := range []struct {
		index map[string]Any
		name  string
		what  string
	}{{r.byKey, s.Key(), "key"}, {r.byEnv, s.Env(), "env"}, {r.byFlag, s.Flag(), "flag"}} {
		if n.name == "" {
			continue
		}
		if prev, dup := n.index[n.name]; dup {
			return fmt.Errorf("config: %s %q is declared twice (%s and %s)", n.what, n.name, prev.Key(), s.Key())
		}
	}
	r.byKey[s.Key()] = s
	r.byEnv[s.Env()] = s
	if s.Flag() != "" {
		r.byFlag[s.Flag()] = s
	}
	r.ordered = append(r.ordered, s)
	return nil
}

func wellFormed(s Any) error {
	switch {
	case !keyForm.MatchString(s.Key()):
		return fmt.Errorf("config: key %q is not lowercase and dot-separated", s.Key())
	case !envForm.MatchString(s.Env()):
		return fmt.Errorf("config: %s: env name %q is not upper snake case", s.Key(), s.Env())
	case s.Flag() != "" && !flagForm.MatchString(s.Flag()):
		return fmt.Errorf("config: %s: flag %q is not lowercase kebab case", s.Key(), s.Flag())
	case reserved[s.Env()] || reserved[s.Flag()]:
		return fmt.Errorf("config: %s: %s/%s is read by the loader and cannot be a setting", s.Key(), s.Env(), s.Flag())
	case s.Doc() == "":
		return fmt.Errorf("config: %s has no doc", s.Key())
	}
	if v, ok := s.(interface{ invariantFault() error }); ok {
		return v.invariantFault()
	}
	return nil
}

// All lists the settings sorted by key.
func (r *Registry) All() []Any { return slices.Clone(r.ordered) }

// Lookup finds a setting by key.
//
//nolint:ireturn // the registry holds settings of many types behind the erased Any
func (r *Registry) Lookup(key string) (Any, bool) {
	s, ok := r.byKey[key]
	return s, ok
}
