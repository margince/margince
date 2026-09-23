// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What `worker aitask` accepts on the command line, and what it refuses before
// anything runs.
//
// Split from the dispatch in aitask.go because the two answer different
// questions and only one of them is read by the help: aitaskhelp.go renders the
// flag set aiTaskFlagSet builds, so this file is the single statement of what
// the probe verbs take.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/cliflags"
	"github.com/margince/margince/backend/internal/platform/config"
)

type aiTaskFlags struct {
	verb string
	// arg is the verb's positional: the site for scaffold, the URL for fetch.
	arg string

	site         string
	scenarioPath string
	fixturePath  string
	expectPath   string

	modelSpec string
	fakeBrain bool

	jsonPath  string
	dumpDir   string
	corpusDir string
	workDir   string
	outPath   string

	logLevel  string
	logFormat string
}

// aiTaskFlagSet builds the flag set one verb parses with, and the values it
// parses into.
//
// It is a function rather than inline in the parser because help renders the
// SAME set (aitaskhelp.go): a flag added here is documented the day it is
// added, instead of the day somebody remembers a usage block exists.
//
// Its output is discarded and its usage suppressed. This tool prints its own
// help, and `flag`'s would otherwise print a second, poorer one beside it every
// time an argument is mistyped — and print it into the stderr of a container,
// which is why cliflags exists at all.
func aiTaskFlagSet(verb string) (*flag.FlagSet, *aiTaskFlags, *cliflags.Env) {
	cfg := &aiTaskFlags{verb: verb}
	env := &cliflags.Env{}
	fs := flag.NewFlagSet("worker aitask "+verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.StringVar(&cfg.site, "site", "", "invocation site as <task>/<variant> (e.g. rate_extract/pricing)")
	fs.StringVar(&cfg.scenarioPath, "scenario", "", "scenario file in the corpus format, carrying both fixture and expectation")
	fs.StringVar(&cfg.fixturePath, "fixture", "", "fixture JSON file; needs --site, and --expect for sites that validate one")
	fs.StringVar(&cfg.expectPath, "expect", "", "expected-answer JSON file, the half --fixture does not carry")
	fs.StringVar(&cfg.modelSpec, "model", "", "direct model override, provider:model (e.g. anthropic:claude-sonnet-4-6)")
	fs.BoolVar(&cfg.fakeBrain, "ai-fake", false, "offline fake model: drives the seam without spending anything")
	fs.StringVar(&cfg.jsonPath, "json", "", "write the machine-readable probe result here ('-' = stdout)")
	fs.StringVar(&cfg.dumpDir, "dump-request", "", "directory to write each post-stripper request into")
	fs.StringVar(&cfg.corpusDir, "corpus", corpusDirDefault, "corpus directory, read by list and scaffold")
	env.String(fs, &cfg.workDir, "work-dir", "MARGINCE_AITASK_DIR", workDirDefault,
		"gitignored directory probe artifacts are written to; they carry whatever the probed source carried")
	fs.StringVar(&cfg.outPath, "out", "", "write this verb's artifact here instead of the work directory ('-' = stdout)")
	env.String(fs, &cfg.logLevel, "log-level", "MARGINCE_LOG_LEVEL", "info", "log level: debug|info|warn|error")
	env.String(fs, &cfg.logFormat, "log-format", "MARGINCE_LOG_FORMAT", "text", "log format: text|json")
	return fs, cfg, env
}

func parseAITaskFlags(args []string) (aiTaskFlags, error) {
	if len(args) == 0 {
		return aiTaskFlags{}, fmt.Errorf("aitask needs a verb: %s", strings.Join(aiTaskVerbNames(), ", "))
	}
	if !slices.Contains(probeFlagVerbs, args[0]) {
		return aiTaskFlags{}, fmt.Errorf("aitask: unknown verb %q — want %s",
			args[0], strings.Join(probeFlagVerbs, ", "))
	}
	fs, parsed, env := aiTaskFlagSet(args[0])

	// stdlib flag stops at the first positional; re-parsing the remainder lets
	// the positional and the flags interleave, as siteread's seeds do.
	rest := args[1:]
	var positionals []string
	for {
		if err := fs.Parse(rest); err != nil {
			return aiTaskFlags{}, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positionals = append(positionals, rest[0])
		rest = rest[1:]
	}
	env.Apply(fs, config.FromOS)
	cfg := *parsed
	if len(positionals) > 1 {
		return aiTaskFlags{}, fmt.Errorf("aitask %s takes one positional, got %d: %s",
			cfg.verb, len(positionals), strings.Join(positionals, " "))
	}
	if len(positionals) == 1 {
		cfg.arg = positionals[0]
	}

	if err := cfg.validate(); err != nil {
		return aiTaskFlags{}, err
	}
	return cfg, nil
}

// validate refuses the combinations that could only fail later, and names the
// flag that would fix each — a probe that dies after a paid call on something
// knowable up front has spent money to say nothing.
func (c aiTaskFlags) validate() error {
	switch c.verb {
	case verbScaffold:
		if c.arg == "" && c.site == "" {
			return errors.New("aitask scaffold needs a site: <task>/<variant>, e.g. rate_extract/pricing")
		}
	case verbFetch:
		if c.arg == "" {
			return errors.New("aitask fetch needs a url")
		}
	case verbRun:
		switch {
		case c.scenarioPath == "" && c.fixturePath == "":
			return errors.New("aitask run needs --scenario or --fixture")
		case c.scenarioPath != "" && c.fixturePath != "":
			return errors.New("aitask run takes --scenario or --fixture, not both — they disagree about what is being probed")
		case c.fixturePath != "" && c.siteRef() == "":
			return errors.New("aitask run --fixture needs --site: a fixture names no site, and only the site says which code probes it")
		case c.scenarioPath != "" && c.expectPath != "":
			// A scenario carries its own expectation. Taking --expect too would
			// silently grade against one of them, and the expectation decides
			// the verdict.
			return errors.New("aitask run takes --expect with --fixture, not with --scenario: a scenario already carries its expectation")
		}
	}
	return nil
}

// artifactOut is where this verb's artifact goes: the operator's --out when
// they named one, otherwise the gitignored work directory under the given name.
func (c aiTaskFlags) artifactOut(name string) string {
	return artifactOutOr(c.outPath, c.workDir, name)
}

// artifactOutOr answers "--out wins, otherwise the work directory". Both flag
// sets reach it — `retrieve` parses with its own and owes the same answer — so
// neither restates the rule and they cannot come to disagree about it.
func artifactOutOr(outPath, workDir, name string) string {
	if outPath != "" {
		return outPath
	}
	return artifactPath(workDir, name)
}

// siteRef is the site the run is bound to, from either spelling.
func (c aiTaskFlags) siteRef() string {
	if c.site != "" {
		return c.site
	}
	return c.arg
}
