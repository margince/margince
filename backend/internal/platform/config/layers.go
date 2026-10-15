// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// layer is one source's candidates keyed by setting key: text from a flag or
// variable, a node from YAML. names holds what the operator wrote, and vars
// every variable an environment source held, for the typo report.
type layer struct {
	source Source
	file   string
	text   map[string]string
	nodes  map[string]*yaml.Node
	names  map[string]string
	vars   map[string]string
}

func locate(flags flagValues, env map[string]string) (Files, bool, error) {
	files := Files{
		Base:       cmp.Or(flags.config, env[envConfig], "margince.yaml"),
		ConfigFile: cmp.Or(flags.configFile, env[envConfigFile]),
		EnvFile:    cmp.Or(flags.envFile, env[envEnvFile]),
	}
	if flags.trace || env[envTrace] == "" {
		return files, flags.trace, nil
	}
	on, err := strconv.ParseBool(env[envTrace])
	if err != nil {
		return files, false, fmt.Errorf("%s (%s): want true or false", envTrace, SourceEnv)
	}
	return files, on, nil
}

// readLayers returns the layers highest first, so the first candidate wins.
func readLayers(reg *Registry, src Sources, flags flagValues, env map[string]string, files Files) ([]layer, []error, []string) {
	flagLayer := textLayer(SourceFlag, "", flags.raw, reg.byFlag, func(name string) string { return "--" + name })
	envLayer := textLayer(SourceEnv, "", env, reg.byEnv, nil)
	envLayer.vars = env
	layers := []layer{flagLayer, envLayer}
	trace := []string{
		fmt.Sprintf("config: read %s %d", SourceFlag, len(flagLayer.text)),
		fmt.Sprintf("config: read %s %d", SourceEnv, len(envLayer.text)),
	}
	var faults []error
	add := func(l layer, errs []error) {
		layers = append(layers, l)
		faults = append(faults, errs...)
		trace = append(trace, fmt.Sprintf("config: read %s %s", l.source, l.file))
	}
	if files.EnvFile != "" {
		add(envFileLayer(reg, src.ReadFile, files.EnvFile))
	}
	if files.ConfigFile != "" {
		add(yamlFileLayer(SourceConfigFile, files.ConfigFile, src))
	}
	add(yamlFileLayer(SourceBaseFile, files.Base, src))
	for _, path := range overlayBeside(files.Base) {
		faults = append(faults, fmt.Errorf("%s: overlay files are no longer read; pass it with --config-file", path))
	}
	return layers, faults, trace
}

// textLayer drops an empty variable: .env.example ships optional lines as bare
// NAME= placeholders that entrypoints export, and those mean "not set".
func textLayer(source Source, file string, raw map[string]string, index map[string]Any, display func(string) string) layer {
	l := layer{source: source, file: file, text: map[string]string{}, names: map[string]string{}}
	for name, value := range raw {
		s, ok := index[name]
		if !ok || (source != SourceFlag && value == "") {
			continue
		}
		l.text[s.Key()] = value
		l.names[s.Key()] = name
		if display != nil {
			l.names[s.Key()] = display(name)
		}
	}
	return l
}

func envFileLayer(reg *Registry, readFile func(string) ([]byte, error), path string) (layer, []error) {
	empty := layer{source: SourceEnvFile, file: path}
	raw, err := readFile(path)
	if err != nil {
		return empty, []error{fmt.Errorf("%s (%s): %w", path, SourceEnvFile, err)}
	}
	vars, err := parseDotenv(path, raw)
	if err != nil {
		return empty, []error{err}
	}
	var faults []error
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		if why := readOnlyElsewhere(name); why != "" {
			faults = append(faults, fmt.Errorf("%s: %s", path, why))
		}
	}
	l := textLayer(SourceEnvFile, path, vars, reg.byEnv, nil)
	l.vars = vars
	return l, faults
}

// yamlFileLayer refuses a missing --config-file, since an operator named it.
// A missing base file is no fault: a system may run on flags and variables.
func yamlFileLayer(source Source, path string, src Sources) (layer, []error) {
	empty := layer{source: source, file: path}
	if _, err := os.Stat(path); source == SourceConfigFile && errors.Is(err, fs.ErrNotExist) {
		return empty, []error{fmt.Errorf("%s (%s): no such file", path, source)}
	}
	parsed, unknown, err := readYAML(path, src.KnownKey, src.BootstrapRoots)
	if err != nil {
		return empty, []error{err}
	}
	var faults []error
	reported := map[string]bool{}
	for _, key := range unknown {
		root, _, _ := strings.Cut(key, ".")
		why := readOnlyElsewhere(map[string]string{"env": envPosture, "config": envConfig}[root])
		if why == "" {
			why = "unknown key " + key
		}
		if !reported[why] {
			reported[why] = true
			faults = append(faults, fmt.Errorf("%s: %s", path, why))
		}
	}
	l := layer{source: source, file: path, nodes: parsed.values, names: map[string]string{}}
	for key := range parsed.values {
		l.names[key] = key
	}
	return l, faults
}

// readOnlyElsewhere explains why a name Load reads before any file cannot sit
// in one: the files it would pick are already chosen by then.
func readOnlyElsewhere(name string) string {
	switch name {
	case envPosture:
		return name + " is read only from the environment"
	case envConfig, envConfigFile, envEnvFile, envTrace:
		return name + " is read only from a flag or the environment"
	}
	return ""
}
