// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// EntrypointVar is a MARGINCE_* variable an image entrypoint script reads
// before it starts a program.
type EntrypointVar struct {
	Name string
	// Required is true when the script stops on an unset or empty value.
	Required bool
	// Script is the first script that reads it, relative to the scanned folder.
	Script string
}

// entrypointRef matches `${NAME...}`. The `:?` and `?` forms stop the script on
// an unset value; `:-default` and a bare `${NAME}` do not. A `$NAME` without
// braces, an explicit guard or `set -u` is not seen.
var entrypointRef = regexp.MustCompile(`\$\{(MARGINCE_[A-Z0-9_]+)(:?\?)?`)

// EntrypointVars reads every script in dir, sorted by name. The generated
// .env.example and the entrypoint gates both read the list from here.
func EntrypointVars(dir string) ([]EntrypointVar, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("deployconfig: reading the entrypoint scripts in %s: %w", dir, err)
	}
	byName := map[string]*EntrypointVar{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- a script of the repository's own deploy folder
		if err != nil {
			return nil, fmt.Errorf("deployconfig: reading %s: %w", e.Name(), err)
		}
		for _, m := range entrypointRef.FindAllStringSubmatch(string(raw), -1) {
			v, seen := byName[m[1]]
			if !seen {
				v = &EntrypointVar{Name: m[1], Script: e.Name()}
				byName[m[1]] = v
			}
			v.Required = v.Required || m[2] != ""
		}
	}
	out := make([]EntrypointVar, 0, len(byName))
	for _, v := range byName {
		out = append(out, *v)
	}
	slices.SortFunc(out, func(a, b EntrypointVar) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}
