// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"flag"
	"fmt"
	"io"
)

// given names the loader's own flags that appeared, so an explicit
// --config-trace=false or --config-file= still beats the environment.
type flagValues struct {
	raw                         map[string]string
	config, configFile, envFile string
	trace                       bool
	given                       map[string]bool
}

// parseFlags keeps every flag as text. The setting's Type parses it with the
// same code that parses env, so a flag and a variable never disagree on a
// value's meaning.
func parseFlags(r *Registry, args []string) (flagValues, error) {
	out := flagValues{raw: map[string]string{}, given: map[string]bool{}}
	fs := newFlagSet(r, &out)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return flagValues{}, err
	}
	if fs.NArg() > 0 {
		return flagValues{}, fmt.Errorf("unexpected argument at position %d; flags take --name=value", len(args)-fs.NArg()+1)
	}
	fs.Visit(func(f *flag.Flag) { out.given[f.Name] = true })
	return out, nil
}

// PrintUsage writes every flag with its doc and never a default: each flag is
// registered without one, so a secret's value cannot reach the text.
func PrintUsage(w io.Writer, r *Registry) {
	fs := newFlagSet(r, &flagValues{raw: map[string]string{}})
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func newFlagSet(r *Registry, out *flagValues) *flag.FlagSet {
	fs := flag.NewFlagSet("margince", flag.ContinueOnError)
	fs.StringVar(&out.config, "config", "", "base YAML file (default margince.yaml)")
	fs.StringVar(&out.configFile, "config-file", "", "YAML file layered over the base file")
	fs.StringVar(&out.envFile, "env-file", "", ".env file to read")
	fs.BoolVar(&out.trace, "config-trace", false, "log every configuration source and each setting's origin")
	for _, s := range r.All() {
		name := s.Flag()
		if name == "" {
			continue
		}
		record := func(v string) error {
			out.raw[name] = v
			return nil
		}
		if s.TypeName() == "bool" {
			fs.BoolFunc(name, s.Doc(), record)
		} else {
			fs.Func(name, s.Doc(), record)
		}
	}
	return fs
}
