// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command mcpe2ecmd says which MCP end-to-end verdicts still describe the
// product. It is a go-run-only developer tool, run from backend/:
//
//	go run ./internal/compose/mcpe2e/mcpe2ecmd report            every verdict, current or stale
//	go run ./internal/compose/mcpe2e/mcpe2ecmd stamps -door mcp|rest -out FILE  the stamps a run files
//	go run ./internal/compose/mcpe2e/mcpe2ecmd current -out FILE   each verdict's key, and whether it is current
//	go run ./internal/compose/mcpe2e/mcpe2ecmd newly-stale -base-current FILE
//	                                    the verdicts this tree makes stale against the base's FILE, as Markdown
//
// The base tree writes `current` with its own code, so a change to how a stamp
// is computed shows. FILE is a JSON object of Verdict.Key to a bool, a format
// both sides of a pull request must read alike.
//
// report and newly-stale always exit 0. The lane is paid and manual, so a stale
// verdict is a hint for a pull request, never a reason to block it.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcpe2ecmd: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: mcpe2ecmd report | stamps -door mcp|rest -out FILE | current -out FILE | newly-stale -base-current FILE")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	root := flags.String("root", "..", "the repository root (relative to backend/)")
	out := flags.String("out", "", "stamps, current: the file to write")
	baseCurrent := flags.String("base-current", "", "newly-stale: the file `current` wrote in the base tree")
	door := flags.String("door", string(mcpe2e.DoorMCP), "stamps: the door the run reaches Margince through (mcp or rest)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	var text string
	switch args[0] {
	case "report":
		lines, err := mcpe2e.Report(*root)
		if err != nil {
			return err
		}
		text = mcpe2e.Summarise(lines)
	case "stamps":
		if *out == "" {
			return errors.New("stamps needs -out")
		}
		return mcpe2e.WriteStamps(*root, mcpe2e.Door(*door), *out)
	case "current":
		if *out == "" {
			return errors.New("current needs -out")
		}
		return writeCurrency(*root, *out)
	case "newly-stale":
		if *baseCurrent == "" {
			return errors.New("newly-stale needs -base-current")
		}
		lines, err := newlyStale(*baseCurrent, *root)
		if err != nil {
			return err
		}
		text = newlyStaleMarkdown(lines)
	default:
		return errors.New("unknown command " + args[0])
	}
	_, err := io.WriteString(stdout, text)
	return err
}

func writeCurrency(root, path string) error {
	currency, err := mcpe2e.Currency(root)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(currency, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func newlyStale(baseCurrentPath, root string) ([]mcpe2e.Line, error) {
	body, err := os.ReadFile(baseCurrentPath) // #nosec G304 -- a path the workflow passes, read by a developer tool
	if err != nil {
		return nil, err
	}
	var baseCurrent map[string]bool
	if err := json.Unmarshal(body, &baseCurrent); err != nil {
		return nil, fmt.Errorf("the base's currency %s: %w", baseCurrentPath, err)
	}
	if baseCurrent == nil {
		return nil, fmt.Errorf("the base's currency %s holds no object", baseCurrentPath)
	}
	return mcpe2e.NewlyStale(baseCurrent, root)
}

func newlyStaleMarkdown(lines []mcpe2e.Line) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This change makes %d MCP end-to-end verdicts stale. They no longer describe what "+
		"the assistant is served.\n\n", len(lines))
	b.WriteString("| Verdict | Scenario | What moved |\n|---|---|---|\n")
	for _, line := range lines {
		fmt.Fprintf(&b, "| `%s/%s` | `%s` | %s |\n", line.Tree, line.Folder, line.Scenario, strings.Join(line.Why, ", "))
	}
	b.WriteString("\nThis reports and never blocks. A paid run of these cases (`make e2e-llm`) " +
		"stamps them again; list them in the PR body if you do not run it.\n")
	return b.String()
}
