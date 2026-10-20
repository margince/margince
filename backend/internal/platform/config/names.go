// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/editdistance"
)

// Names are setting keys and the variables that set them.
type Names struct{ Keys, Envs []string }

// suggestLimit is how far a misspelling may be from the name it meant. Two
// edits catch a dropped letter or a swapped pair; more suggests names nobody
// meant.
const suggestLimit = 2

// Suggest is the name among candidates that name most likely misspells, or ""
// when none is close enough to offer.
func Suggest(name string, candidates []string) string {
	near, _ := editdistance.Nearest(name, candidates, suggestLimit)
	return near
}

// UnknownKeyFault reports a YAML key no reader takes. The settings loader and
// the bootstrap reader both report through it, so the two read alike.
func UnknownKeyFault(file, key string, line int, near string) error {
	msg := fmt.Sprintf("%s: unknown key %s (line %d)", file, key, line)
	if near != "" {
		msg += "; did you mean " + near + "?"
	}
	return errors.New(msg)
}

// known holds the names Load takes without a fault or a warning.
type known struct {
	keys, envs map[string]bool
	// sections are the dotted prefixes of the keys. A mapping under one may
	// hold a key; a mapping under anything else is unknown whole.
	sections       map[string]bool
	bootstrapRoots map[string]bool
}

func knownNames(reg *Registry, others Names, bootstrapRoots map[string]bool) known {
	k := known{keys: map[string]bool{}, envs: map[string]bool{}, sections: map[string]bool{}, bootstrapRoots: bootstrapRoots}
	for _, s := range reg.All() {
		k.keys[s.Key()] = true
		k.envs[s.Env()] = true
	}
	for _, key := range others.Keys {
		k.keys[key] = true
	}
	for _, env := range others.Envs {
		k.envs[env] = true
	}
	for key := range k.keys {
		for i, c := range key {
			if c == '.' {
				k.sections[key[:i]] = true
			}
		}
	}
	return k
}

// nearKey compares a key with the names at its own depth, so a misspelt
// section gets a section and a misspelt leaf gets a leaf.
func (k known) nearKey(key string) string {
	depth := strings.Count(key, ".")
	var candidates []string
	for _, names := range []map[string]bool{k.keys, k.sections} {
		for name := range names {
			if strings.Count(name, ".") == depth {
				candidates = append(candidates, name)
			}
		}
	}
	if depth == 0 {
		candidates = append(candidates, slices.Collect(maps.Keys(k.bootstrapRoots))...)
	}
	return Suggest(key, candidates)
}

// nearEnv also offers the loader's own variables, which no setting declares.
func (k known) nearEnv(name string) string {
	candidates := slices.Collect(maps.Keys(k.envs))
	for _, v := range loaderVars {
		if v.Env != "" {
			candidates = append(candidates, v.Env)
		}
	}
	return Suggest(name, candidates)
}
