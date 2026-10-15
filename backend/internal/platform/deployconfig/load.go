// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// document is a whole margince.yaml: the bootstrap roots, strictly decoded,
// beside every other root, which config.Load reads and this package skips.
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

// Parse strictly decodes and validates one document's bootstrap roots.
func Parse(raw []byte) (Bootstrap, error) {
	boot, _, err := decode(raw)
	if err != nil {
		return Bootstrap{}, fmt.Errorf("deployconfig: %w", err)
	}
	return boot, boot.validate()
}

// LoadBootstrap reads the bootstrap roots of one file. A missing file is an
// installation that is already bootstrapped and needs none.
func LoadBootstrap(path string) (Bootstrap, error) { return LoadBootstrapFiles(path, "") }

// LoadBootstrapFiles reads the base file, then the --config-file over it. A
// root the config file writes replaces the base's whole, so a dev stack's
// seeds.ai_routing is the dev file's own; validation runs once, over the
// result. A missing file is skipped here, since config.Load refuses a missing
// --config-file before this runs.
func LoadBootstrapFiles(base, configFile string) (Bootstrap, error) {
	boot := Bootstrap{Version: 1}
	for _, path := range []string{base, configFile} {
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- the operator's own --config / --config-file path
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Bootstrap{}, fmt.Errorf("deployconfig: reading %s: %w", path, err)
		}
		layer, written, err := decode(raw)
		if err != nil {
			return Bootstrap{}, fmt.Errorf("deployconfig: %s: %w", path, err)
		}
		replaceRoots(&boot, layer, written)
	}
	return boot, boot.validate()
}

// decode returns the bootstrap roots of one document and which of them it
// writes. A file holding only comments is an empty document.
func decode(raw []byte) (Bootstrap, map[string]bool, error) {
	doc := document{Bootstrap: Bootstrap{Version: 1}}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return Bootstrap{}, nil, err
	}
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &keys); err != nil {
		return Bootstrap{}, nil, err
	}
	written := map[string]bool{}
	for root := range Roots() {
		if _, ok := keys[root]; ok {
			written[root] = true
		}
	}
	return doc.Bootstrap, written, nil
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
