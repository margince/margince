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
	"os"

	"github.com/margince/margince/backend/internal/compose"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: presetbodycmd <preset.yaml>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1]) // #nosec G304 G703 -- the preset the operator named on the command line
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := compose.PresetRoutingBody(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	if _, err := fmt.Fprintf(os.Stdout, "%s\n", out); err != nil {
		fmt.Fprintf(os.Stderr, "presetbodycmd: writing the body: %v\n", err)
		os.Exit(1)
	}
}
