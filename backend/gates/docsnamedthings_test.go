// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A page that tells the reader to run a make target, set an environment
// variable or follow a contribution rule is a claim about the tree. These are
// checked against their owners: the Makefiles, the non-Markdown sources, and
// the short list of instructions the project has retired.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	namedMakeInline = regexp.MustCompile("`make ([a-z][a-z0-9_.-]*[a-z0-9])[ `]")
	namedMakeFenced = regexp.MustCompile(`^\s*(?:\$\s*)?make ([a-z][a-z0-9_.-]*[a-z0-9])\b`)
	namedEnv        = regexp.MustCompile(`MARGINCE_[A-Z0-9_]*[A-Z0-9](\*|_\*|_<)?`)
	makeRule        = regexp.MustCompile(`(?m)^([A-Za-z0-9_.%/-]+)\s*:([^=]|$)`)
	makePhony       = regexp.MustCompile(`(?m)^\.PHONY:(.*)$`)
)

// shellFenceLang are the fences whose lines are commands; a prompt or a
// transcript in a plain fence may start a sentence with "make".
var shellFenceLang = map[string]bool{"sh": true, "bash": true, "shell": true, "console": true, "zsh": true}

// retiredInstructions are rules the project dropped. A page still teaching one
// sends a contributor through a step nothing checks any more.
var retiredInstructions = []struct {
	pattern *regexp.Regexp
	why     string
}{
	{
		regexp.MustCompile(`(?i)git commit -s\b|--signoff|Signed-off-by|\b(commits?|every commit) (is|are) signed off|sign off (your|every|each) commit|\bDCO\b`),
		"commit sign-off (DCO) was retired; no workflow checks it",
	},
}

func makeTargets(t *testing.T) map[string]bool {
	t.Helper()
	targets := map[string]bool{}
	for _, mk := range []string{"Makefile", "backend/Makefile"} {
		raw, err := os.ReadFile(filepath.Join(docsTreeRoot, mk))
		if err != nil {
			t.Fatalf("read %s: %v", mk, err)
		}
		for _, m := range makeRule.FindAllStringSubmatch(string(raw), -1) {
			targets[m[1]] = true
		}
		for _, m := range makePhony.FindAllStringSubmatch(string(raw), -1) {
			for _, name := range strings.Fields(m[1]) {
				targets[name] = true
			}
		}
	}
	return targets
}

// sourceEnvNames collects every MARGINCE_ name a non-Markdown source spells, so
// a variable a page documents must be read, set or defaulted somewhere real.
func sourceEnvNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, f := range trackedFiles(t) {
		if f.symlink || strings.HasSuffix(f.path, ".md") || strings.HasPrefix(f.path, "frontend/node_modules/") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(docsTreeRoot, f.path))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", f.path, err)
		}
		for _, m := range namedEnv.FindAllString(string(raw), -1) {
			names[strings.TrimRight(m, "*_<")] = true
		}
	}
	return names
}

func envKnown(name string, known map[string]bool) bool {
	if known[name] {
		return true
	}
	for k := range known {
		if strings.HasPrefix(k, name+"_") {
			return true
		}
	}
	return false
}

func TestDocsNameOnlyThingsThatExist(t *testing.T) {
	t.Parallel()
	targets, envs := makeTargets(t), sourceEnvNames(t)
	if len(targets) < 50 || len(envs) < 20 {
		t.Fatalf("found %d make targets and %d env names; the owners moved, so this gate reads nothing", len(targets), len(envs))
	}
	for _, rel := range prosePages(t) {
		raw, err := os.ReadFile(filepath.Join(docsTreeRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		inFence, shellFence := false, false
		for i, line := range strings.Split(string(raw), "\n") {
			if trimmed := strings.TrimSpace(line); barFence.MatchString(trimmed) {
				inFence = !inFence
				shellFence = inFence && shellFenceLang[strings.TrimLeft(trimmed, "`~")]
				continue
			}
			var named [][]string
			if shellFence {
				named = namedMakeFenced.FindAllStringSubmatch(line, -1)
			} else if !inFence {
				named = namedMakeInline.FindAllStringSubmatch(line+" ", -1)
			}
			for _, m := range named {
				if !targets[m[1]] {
					t.Errorf("%s:%d names `make %s`, which no Makefile defines", rel, i+1, m[1])
				}
			}
			for _, m := range namedEnv.FindAllStringSubmatch(line, -1) {
				if name := strings.TrimRight(m[0], "*_<"); m[1] == "" && !envKnown(name, envs) {
					t.Errorf("%s:%d names %s, which no source reads or sets", rel, i+1, name)
				}
			}
			for _, r := range retiredInstructions {
				if r.pattern.MatchString(line) {
					t.Errorf("%s:%d teaches a retired instruction: %s", rel, i+1, r.why)
				}
			}
		}
	}
}

func TestDocsNamedThingsSeePlantedClaims(t *testing.T) {
	t.Parallel()
	if m := namedMakeInline.FindStringSubmatch("run `make check-go` first "); m == nil || m[1] != "check-go" {
		t.Errorf("inline make target not recognised: %v", m)
	}
	if m := namedMakeFenced.FindStringSubmatch("$ make db-up"); m == nil || m[1] != "db-up" {
		t.Errorf("fenced make target not recognised: %v", m)
	}
	if !retiredInstructions[0].pattern.MatchString("Commit with `git commit -s`.") {
		t.Error("a retired sign-off instruction went unrecognised")
	}
	if retiredInstructions[0].pattern.MatchString("Type your sign-off in **Sign-off**.") {
		t.Error("an email sign-off was read as the retired commit rule")
	}
}
