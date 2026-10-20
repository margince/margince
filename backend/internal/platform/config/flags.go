// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// given names the loader's own flags that appeared, so an explicit
// --config-trace=false or --config-file= still beats the environment.
type flagValues struct {
	raw                         map[string]string
	config, configFile, envFile string
	trace, check                bool
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
		return flagValues{}, FlagFault(fs, err)
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
	fs.StringVar(&out.config, "config", "", loaderDoc("config"))
	fs.StringVar(&out.configFile, "config-file", "", loaderDoc("config-file"))
	fs.StringVar(&out.envFile, "env-file", "", loaderDoc("env-file"))
	fs.BoolVar(&out.trace, "config-trace", false, loaderDoc("config-trace"))
	fs.BoolVar(&out.check, checkFlag, false, loaderDoc(checkFlag))
	for _, s := range r.All() {
		addFlag(fs, s, func(v string) { out.raw[s.Flag()] = v })
	}
	return fs
}

// addFlag registers s's flag with no default, so no value can reach usage text.
// A bool flag may stand alone: a bare --inline-relay means true.
func addFlag(fs *flag.FlagSet, s Any, record func(string)) {
	name := s.Flag()
	if name == "" {
		return
	}
	set := func(v string) error {
		record(v)
		return nil
	}
	if s.TypeName() == Bool.Name() {
		fs.BoolFunc(name, s.Doc(), set)
	} else {
		fs.Func(name, s.Doc(), set)
	}
}

// CommandFlags carries a registry's flags on a command's own flag set. The
// command also takes flags Load does not know, such as migrate's --steps.
type CommandFlags struct{ given []string }

// AddFlags registers the loader's flags and r's on fs, without defaults, as
// PrintUsage does.
func AddFlags(fs *flag.FlagSet, r *Registry) *CommandFlags {
	c := &CommandFlags{}
	for _, v := range loaderVars {
		// A command with flags of its own runs its own verb, which a dry run of
		// the configuration would skip in silence.
		if v.Flag == "" || v.Flag == checkFlag {
			continue
		}
		record := func(value string) error {
			c.given = append(c.given, "--"+v.Flag+"="+value)
			return nil
		}
		if v.Flag == "config-trace" {
			fs.BoolFunc(v.Flag, v.Doc, record)
		} else {
			fs.Func(v.Flag, v.Doc, record)
		}
	}
	for _, s := range r.All() {
		addFlag(fs, s, func(v string) { c.given = append(c.given, "--"+s.Flag()+"="+v) })
	}
	return c
}

// Args are the registry flags the command line gave, as Load's Args, so Load
// still parses each value and ranks it against the environment.
func (c *CommandFlags) Args() []string { return c.given }

// undefinedFlag is how the flag package words an unknown flag. It names the
// flag with one dash, whichever the command line used.
const undefinedFlag = "flag provided but not defined: -"

// FlagFault adds to fs's parse error the flag an unknown one likely meant.
func FlagFault(fs *flag.FlagSet, err error) error {
	name, ok := strings.CutPrefix(err.Error(), undefinedFlag)
	if !ok {
		return err
	}
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	if near := Suggest(name, names); near != "" {
		return fmt.Errorf("%w; did you mean --%s?", err, near)
	}
	return err
}
