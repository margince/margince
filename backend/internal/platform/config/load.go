// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// Sources are what Load reads. Args holds flags only: a program strips its
// positional subcommand before it calls Load.
type Sources struct {
	Args    []string
	Environ []string
	// Known holds every program's names, so a key or variable a sibling
	// program reads is neither a fault nor a warning here. This registry's own
	// names are always known.
	Known          Names
	BootstrapRoots map[string]bool
	ReadFile       func(path string) ([]byte, error)
	// Faults checks what reads two settings. It runs even when a setting
	// faulted; that setting then reads as its zero value from SourceUnset.
	Faults func(*Config) []error
}

// Files are the paths Load used, after flags and the environment chose them.
type Files struct{ Base, ConfigFile, EnvFile string }

type resolved struct {
	value  any //craft:ignore naked-any one map holds the values of settings of many types
	origin Origin
	set    bool
}

// Config holds the values Load resolved from the boot sources.
type Config struct {
	reg       *Registry
	values    map[string]resolved
	trace     []string
	tracing   bool
	warnings  []string
	elsewhere []string
	files     Files
	db        *settings.Store
	sealer    *Sealer
}

// Load resolves every setting and returns all faults joined, so an operator
// fixes a broken configuration in one pass instead of one boot per fault.
func Load(reg *Registry, src Sources) (*Config, error) {
	cfg, err := loadSources(reg, src)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadSources returns what it resolved beside the faults, for Check to trace. The
// Config is nil only when the command line does not parse.
func loadSources(reg *Registry, src Sources) (*Config, error) {
	names := knownNames(reg, src.Known, src.BootstrapRoots)
	if src.ReadFile == nil {
		src.ReadFile = os.ReadFile
	}
	env := environMap(src.Environ)
	flags, err := parseFlags(reg, src.Args)
	if err != nil {
		return nil, fmt.Errorf("command line: %w", err)
	}
	var faults []error
	files, tracing, err := locate(flags, env)
	if err != nil {
		faults = append(faults, err)
	}
	layers, layerFaults, trace := readLayers(reg, src, names, flags, env, files)
	faults = append(faults, layerFaults...)
	cfg := &Config{reg: reg, values: map[string]resolved{}, files: files, trace: trace, tracing: tracing}
	getenv := func(name string) (string, bool) {
		v, ok := env[name]
		return v, ok
	}
	for _, s := range reg.All() {
		r, err := resolve(s, layers, src.ReadFile, getenv)
		if err != nil {
			faults = append(faults, err)
			r = resolved{origin: Origin{Source: SourceUnset}}
		}
		cfg.values[s.Key()] = r
	}
	cfg.warnings = undeclared(names, layers)
	cfg.elsewhere = readElsewhere(reg, names, layers)
	if src.Faults != nil {
		faults = append(faults, src.Faults(cfg)...)
	}
	return cfg, errors.Join(faults...)
}

// environMap keeps the last of a repeated name, as os.Getenv does.
func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, entry := range environ {
		if name, value, ok := strings.Cut(entry, "="); ok {
			out[name] = value
		}
	}
	return out
}

// EnvironValue is name's value in environ, read as os.Getenv reads a repeat.
// A program reads the posture this way before Load, for its Faults hook.
func EnvironValue(environ []string, name string) string { return environMap(environ)[name] }

// Warnings names the MARGINCE_* variables that no program reads, each with the
// name it likely meant, and never their values.
func (c *Config) Warnings() []string { return c.warnings }

// Files are the paths Load read.
func (c *Config) Files() Files { return c.files }

// Registry is the set of settings this Config resolved.
func (c *Config) Registry() *Registry { return c.reg }

// Trace is whether --config-trace or MARGINCE_CONFIG_TRACE asked for a trace.
func (c *Config) Trace() bool { return c.tracing }
