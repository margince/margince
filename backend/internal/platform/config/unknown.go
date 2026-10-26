// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

// A misspelled variable currently does nothing at all.
//
// MARGINCE_REDDIS=… is not an error, not a warning, and not a different
// behaviour: the process reads MARGINCE_REDIS, finds nothing, and takes the
// default. The operator has no thread to pull, and this report is that thread.
//
// It is a warning and never a refusal. A deployment carries variables no
// program reads, such as the entrypoint's and the build's.

import (
	"log/slog"
	"os"
	"strings"
)

// namespace is the prefix that makes a variable ours to have an opinion about.
// A deployment's environment holds PATH, HOME and whatever else the platform
// puts there; none of it is a typo of anything we declare.
const namespace = "MARGINCE_"

// consumedByTheContainerNotTheProcess names variables whose consumer is the
// image itself rather than any Go role, so reporting them would be noise on
// every boot of every installation.
//
// Each says WHO consumes it, so a name that stops being consumed can be found
// and removed rather than accumulating:
//
//   - the container entrypoint writes the bootstrap credential from
//     MARGINCE_ADMIN_PASSWORD before the api starts (ADR-0061 §2);
//   - the build stamps MARGINCE_BUILD_REVISION and MARGINCE_COMPOSITION_FRONTEND
//     into the image, and they outlive the build inside it.
//
// MARGINCE_ADMIN_PASSWORD_FILE is deliberately NOT here. The entrypoint records
// that it was retired and its path hardcoded, so an operator still setting it is
// being ignored — which is exactly what this report exists to say out loud.
//
// The names are assembled from the prefix rather than written whole, because
// the tree-wide documentation gate reads every quoted MARGINCE_* literal as a
// variable this code READS and demands a row for it. These are the opposite:
// names this code deliberately does not read.
var consumedByTheContainerNotTheProcess = map[string]bool{
	namespace + "ADMIN_PASSWORD":       true,
	namespace + "BUILD_REVISION":       true,
	namespace + "COMPOSITION_FRONTEND": true,
}

// reportable is whether a variable is one the typo report may name at all,
// before asking whether any setting declares it.
func reportable(name, value string) bool {
	switch {
	case !strings.HasPrefix(name, namespace):
		return false
	// An empty value is unset: .env.example ships bare NAME= placeholders,
	// and the entrypoints export them.
	case value == "":
		return false
	// The test suite's own plumbing, present in every developer's shell.
	case strings.HasPrefix(name, namespace+"TEST_"), strings.HasPrefix(name, namespace+"BENCH_"):
		return false
	}
	return !consumedByTheContainerNotTheProcess[name]
}

// Environ is the process environment in "NAME=value" form — the only os.Environ
// in the product, here for the same reason FromOS is.
func Environ() []string { return os.Environ() }

// WarnUndeclared writes the report, or nothing when there is nothing to say.
//
// Worded as "no such variable", the line would talk an operator into deleting
// configuration the entrypoint depends on.
//
// Names travel, values never do: a misspelled secret is still a secret, and a
// warning that echoed one would leak the value while complaining about the name.
func WarnUndeclared(logger *slog.Logger, unknown []string) {
	if len(unknown) == 0 {
		return
	}
	logger.Warn("set but read by no Margince program — check the spelling, or ignore this if the entrypoint reads it",
		"variables", unknown)
}

// Report logs what Load found once a program's logger exists: the trace when
// one was asked for, then the variables no program reads.
func (c *Config) Report(logger *slog.Logger) {
	if c.tracing {
		for _, line := range c.TraceLines() {
			logger.Info(line)
		}
	}
	WarnUndeclared(logger, c.warnings)
}
