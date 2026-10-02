// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command presetbodycmd prints a preset's seeds.ai_routing as the JSON body
// PUT /v1/ai/routing takes, so a running stack can be bound to a committed
// preset the way an operator binds one, through the endpoint and nothing else.
//
//	go run ./internal/compose/aicert/presetbodycmd config/presets/<name>.yaml
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/margince/margince/backend/internal/compose"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the command with its arguments and streams passed in: 0 when the body
// was printed, 2 on a usage error, and 1 for any other failure — a file that
// cannot be read, one that is not a preset the product would accept, or a
// body that cannot be written.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return fail(stderr, 2, "usage: presetbodycmd <preset.yaml>")
	}
	raw, err := os.ReadFile(args[0]) // #nosec G304 G703 -- the preset the operator named on the command line
	if err != nil {
		return fail(stderr, 1, err.Error())
	}
	out, err := compose.PresetRoutingBody(raw)
	if err != nil {
		return fail(stderr, 1, fmt.Sprintf("%s: %v", args[0], err))
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", out); err != nil {
		return fail(stderr, 1, "presetbodycmd: writing the body: "+err.Error())
	}
	return 0
}

// fail reports why the command stopped and returns its exit code. A report
// that cannot be written has nowhere left to go, so it can only turn a usage
// error into a plain failure.
func fail(stderr io.Writer, code int, message string) int {
	if _, err := fmt.Fprintln(stderr, message); err != nil {
		return 1
	}
	return code
}
