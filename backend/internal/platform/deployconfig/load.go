// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

// document is a whole margince.yaml: the bootstrap roots beside every other
// root, which config.Load reads and this package skips.
type document struct {
	Bootstrap `yaml:",inline"`
	Settings  map[string]yaml.Node `yaml:",inline"`
}

// Roots are the top-level keys this package reads. config.Load leaves them
// here; the set comes from Bootstrap's own tags, so no list beside it drifts.
func Roots() map[string]bool {
	out := map[string]bool{}
	for field := range reflect.TypeFor[Bootstrap]().Fields() {
		if name := yamlName(field); name != "" {
			out[name] = true
		}
	}
	return out
}

func yamlName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	if name == "-" {
		return ""
	}
	return name
}

// LoadBootstrap reads the bootstrap roots of one file. A missing file is an
// installation that is already bootstrapped and needs none.
func LoadBootstrap(path string) (Bootstrap, error) { return LoadBootstrapFiles(path, "") }

// LoadBootstrapFiles reads the base file, then the --config-file over it. A
// root the config file writes replaces the base's whole, so a dev stack's
// seeds.ai_routing is the dev file's own. Validation runs once, over the
// result, and every fault of both files is returned together. A missing file
// is skipped here, since config.Load refuses a missing --config-file first.
func LoadBootstrapFiles(base, configFile string) (Bootstrap, error) {
	boot, faults, unreadable := read(base, configFile)
	return boot, errors.Join(append(unreadable, faults...)...)
}

// Faults are the faults of the bootstrap roots of the files Load read, for the
// boot to report beside the settings' own. A file that cannot be read or
// parsed is left out, because config.Load already reports it.
func Faults(files config.Files) []error {
	_, faults, _ := read(files.Base, files.ConfigFile)
	return faults
}

// sourceFile is one file the bootstrap roots are read from.
type sourceFile struct {
	path   string
	source config.Source
}

// render words a fault as config.Load words its own, so one boot report reads
// as one list.
func (s sourceFile) render(f fault) error {
	if f.problem == "" {
		return config.UnknownKeyFault(s.path, f.key, f.line, f.near)
	}
	msg := fmt.Sprintf("%s in %s (%s): %s", f.key, s.path, s.source, f.problem)
	if f.line > 0 {
		msg += fmt.Sprintf(" (line %d)", f.line)
	}
	return errors.New(msg)
}

// read returns the faults of the roots apart from the files that could not be
// read at all. A value check is skipped for a root whose keys faulted: a
// misspelt key would otherwise also report the key it meant as missing.
func read(base, configFile string) (Bootstrap, []error, []error) {
	boot := Bootstrap{Version: 1}
	baseFile := sourceFile{base, config.SourceBaseFile}
	origin := map[string]sourceFile{}
	faultedRoots := map[string]bool{}
	var faults, unreadable []error
	for _, file := range []sourceFile{baseFile, {configFile, config.SourceConfigFile}} {
		if file.path == "" {
			continue
		}
		raw, err := os.ReadFile(file.path) // #nosec G304 -- the operator's own --config / --config-file path
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			unreadable = append(unreadable, fmt.Errorf("deployconfig: reading %s: %w", file.path, err))
			continue
		}
		layer, written, keyFaults, err := decode(raw)
		if err != nil {
			unreadable = append(unreadable, fmt.Errorf("deployconfig: %s: %w", file.path, err))
			continue
		}
		for _, f := range keyFaults {
			faults = append(faults, file.render(f))
			faultedRoots[rootOf(f.key)] = true
		}
		replaceRoots(&boot, layer, written)
		for root := range written {
			origin[root] = file
		}
	}
	for _, f := range boot.validate() {
		root := rootOf(f.key)
		if faultedRoots[root] {
			continue
		}
		file, ok := origin[root]
		if !ok {
			file = baseFile
		}
		faults = append(faults, file.render(f))
	}
	return boot, faults, unreadable
}

func rootOf(key string) string {
	root, _, _ := strings.Cut(key, ".")
	return root
}

// decode returns the bootstrap roots of one document, which of them it writes,
// and the faults of their keys. A file holding only comments is an empty
// document. A null root (`seeds:` with its block commented out) is unwritten,
// as config.Load reads a null key.
func decode(raw []byte) (Bootstrap, map[string]bool, []fault, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return Bootstrap{}, nil, nil, err
	}
	if len(root.Content) == 0 {
		return Bootstrap{Version: 1}, nil, nil, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return Bootstrap{}, nil, nil, errors.New("the top level must be a mapping")
	}
	fields := yamlFields(reflect.TypeFor[Bootstrap]())
	written := map[string]bool{}
	var faults []fault
	for i := 0; i+1 < len(top.Content); i += 2 {
		name, value := top.Content[i].Value, top.Content[i+1]
		field, ok := fields[name]
		if !ok {
			continue
		}
		if value.Tag != "!!null" {
			written[name] = true
		}
		faults = append(faults, shapeFaults(name, value, field)...)
	}
	doc := document{Bootstrap: Bootstrap{Version: 1}}
	// The walk above names what a type error would, so a decode that trips on
	// one of those keys adds nothing.
	var typeErr *yaml.TypeError
	if err := root.Decode(&doc); err != nil && !(errors.As(err, &typeErr) && len(faults) > 0) {
		return Bootstrap{}, nil, nil, err
	}
	return doc.Bootstrap, written, faults, nil
}

// replaceRoots copies each root layer writes into boot, whole.
func replaceRoots(boot *Bootstrap, layer Bootstrap, written map[string]bool) {
	dst, src := reflect.ValueOf(boot).Elem(), reflect.ValueOf(layer)
	for i := range dst.NumField() {
		if written[yamlName(dst.Type().Field(i))] {
			dst.Field(i).Set(src.Field(i))
		}
	}
}
