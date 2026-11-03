// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// gen-config writes every file the settings registry is the source of. Those
// are the settings tables in docs/reference/configuration.md, the example
// config, the defaults file, .env.example, the config schema and the routing
// $defs the ai module serves. With -check it writes nothing, names each file that differs and
// exits 1.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// The outputs, relative to the repository root.
const (
	docsPath     = "docs/reference/configuration.md"
	examplePath  = "config/margince.example.yaml"
	defaultsPath = "config/margince.defaults.yaml"
	envPath      = ".env.example"
	schemaPath   = "config/margince.schema.json"
	routingPath  = "backend/internal/modules/ai/routingschema_gen.go"
	// entrypointDir holds the image entrypoint scripts, an input.
	entrypointDir = "scripts/deploy"
	// bootstrapDir is the package of the bootstrap roots, whose field docs
	// describe them in the schema.
	bootstrapDir = "backend/internal/platform/deployconfig"
)

func main() {
	check := flag.Bool("check", false, "write nothing; name each file that differs and exit 1")
	root := flag.String("root", "..", "the repository root, relative to the working directory (backend/)")
	flag.Parse()
	if err := run(*root, *check, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var errStale = errors.New("gen-config: generated configuration files are stale; run `make config-gen`")

func run(root string, check bool, out io.Writer) error {
	page, err := os.ReadFile(filepath.Join(root, docsPath)) // #nosec G304 -- a fixed path under the repository root
	if err != nil {
		return fmt.Errorf("gen-config: reading %s: %w", docsPath, err)
	}
	settings, err := collect(compose.ProgramRegistries())
	if err != nil {
		return err
	}
	entrypoint, err := deployconfig.EntrypointVars(filepath.Join(root, entrypointDir))
	if err != nil {
		return err
	}
	if len(entrypoint) == 0 {
		return fmt.Errorf("gen-config: no MARGINCE_* variable read under %s; the scripts moved", entrypointDir)
	}
	docs, err := readFieldDocs(filepath.Join(root, bootstrapDir), reflect.TypeFor[deployconfig.Bootstrap]().PkgPath())
	if err != nil {
		return err
	}
	files, err := render(settings, string(page), envInputs{loader: config.LoaderVars(), entrypoint: entrypoint}, docs)
	if err != nil {
		return err
	}
	if check {
		stale, err := differing(root, files)
		if err != nil {
			return err
		}
		for _, path := range stale {
			if _, err := fmt.Fprintln(out, path); err != nil {
				return fmt.Errorf("gen-config: %w", err)
			}
		}
		if len(stale) > 0 {
			return errStale
		}
		return nil
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		target := filepath.Join(root, path)
		//nolint:gosec // target is a fixed output path under the -root the developer passed
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("gen-config: creating the directory of %s: %w", path, err)
		}
		//nolint:gosec // as above: a fixed output path under -root
		if err := os.WriteFile(target, files[path], 0o600); err != nil {
			return fmt.Errorf("gen-config: writing %s: %w", path, err)
		}
	}
	return nil
}

// render builds every output from the settings and the current reference
// page, whose prose around the generated region it keeps.
func render(settings []setting, page string, env envInputs, fields fieldDocs) (map[string][]byte, error) {
	docs, err := spliceDocs(page, settings)
	if err != nil {
		return nil, err
	}
	routing, err := routingDefs()
	if err != nil {
		return nil, err
	}
	schema, err := schemaJSON(settings, routing, fields)
	if err != nil {
		return nil, err
	}
	routingGo, err := routingGoFile(routing)
	if err != nil {
		return nil, err
	}
	defaults, err := defaultsYAML(settings)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		docsPath:     []byte(docs),
		examplePath:  []byte(exampleYAML(deployconfig.BootstrapTemplate)),
		defaultsPath: []byte(defaults),
		envPath:      []byte(envExample(settings, env)),
		schemaPath:   schema,
		routingPath:  routingGo,
	}, nil
}

// differing names the outputs whose file on disk is missing or not what
// render produced, sorted.
func differing(root string, files map[string][]byte) ([]string, error) {
	var stale []string
	for _, path := range slices.Sorted(maps.Keys(files)) {
		//nolint:gosec // a fixed output path under the repository root
		current, err := os.ReadFile(filepath.Join(root, path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			stale = append(stale, path)
		case err != nil:
			return nil, fmt.Errorf("gen-config: reading %s: %w", path, err)
		case !bytes.Equal(current, files[path]):
			stale = append(stale, path)
		}
	}
	return stale, nil
}

// setting is one registry setting with what every output needs beside it.
type setting struct {
	config.Any
	shape config.Shape
	// programs are the roles whose registry holds the setting, sorted.
	programs []config.Role
}

// section is the first key segment, or "" for a key without one.
func (s setting) section() string {
	head, _, dotted := strings.Cut(s.Key(), ".")
	if !dotted {
		return ""
	}
	return head
}

// collect joins the registries into one list sorted by key. A setting two
// programs read appears once, naming both.
func collect(regs map[config.Role]*config.Registry) ([]setting, error) {
	byKey := map[string]*setting{}
	for _, role := range slices.Sorted(maps.Keys(regs)) {
		for _, s := range regs[role].All() {
			if seen, ok := byKey[s.Key()]; ok {
				seen.programs = append(seen.programs, role)
				continue
			}
			shape, err := s.Shape()
			if err != nil {
				return nil, err
			}
			byKey[s.Key()] = &setting{Any: s, shape: shape, programs: []config.Role{role}}
		}
	}
	if len(byKey) == 0 {
		return nil, errors.New("gen-config: no program registry holds a setting")
	}
	out := make([]setting, 0, len(byKey))
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, *byKey[key])
	}
	return out, nil
}

// groups splits sorted settings by section, top-level keys first.
func groups(settings []setting) [][]setting {
	var out [][]setting
	var top []setting
	for _, s := range settings {
		switch {
		case s.section() == "":
			top = append(top, s)
		case len(out) > 0 && out[len(out)-1][0].section() == s.section():
			out[len(out)-1] = append(out[len(out)-1], s)
		default:
			out = append(out, []setting{s})
		}
	}
	if len(top) > 0 {
		out = append([][]setting{top}, out...)
	}
	return out
}

// groupTitle names a group in every output.
func groupTitle(group []setting) string {
	if name := group[0].section(); name != "" {
		return name
	}
	return "top-level keys"
}

func programNames(roles []config.Role) string {
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = string(r)
	}
	return strings.Join(names, ", ")
}
