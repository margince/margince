// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command mcpe2ecmd says which MCP end-to-end verdicts still describe the
// product. It is a go-run-only developer tool, run from backend/:
//
//	go run ./internal/compose/mcpe2e/mcpe2ecmd report            every verdict, current or stale
//	go run ./internal/compose/mcpe2e/mcpe2ecmd stamps -out FILE  the stamps a run files (the lane)
//	go run ./internal/compose/mcpe2e/mcpe2ecmd newly-stale -base DIR
//	                                    the verdicts this tree makes stale against DIR, as Markdown
//
// report and newly-stale always exit 0. The lane is paid and manual, so a stale
// verdict is a hint for a pull request, never a reason to block it.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: mcpe2ecmd report | stamps -out FILE | newly-stale -base DIR")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := flags.String("root", "..", "the repository root (relative to backend/)")
	out := flags.String("out", "", "stamps: the file to write")
	base := flags.String("base", "", "newly-stale: the base tree's repository root")
	if err := flags.Parse(os.Args[2:]); err != nil {
		fail(err.Error())
	}
	switch os.Args[1] {
	case "report":
		lines, err := mcpe2e.Report(*root)
		if err != nil {
			fail(err.Error())
		}
		emit(mcpe2e.Summarise(lines))
	case "stamps":
		if *out == "" {
			fail("stamps needs -out")
		}
		if err := mcpe2e.WriteStamps(*root, *out); err != nil {
			fail(err.Error())
		}
	case "newly-stale":
		if *base == "" {
			fail("newly-stale needs -base")
		}
		lines, err := mcpe2e.NewlyStale(*base, *root)
		if err != nil {
			fail(err.Error())
		}
		emit(newlyStaleMarkdown(lines))
	default:
		fail("unknown command " + os.Args[1])
	}
}

func newlyStaleMarkdown(lines []mcpe2e.Line) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This change makes %d MCP end-to-end verdicts stale. They no longer describe what "+
		"the assistant is served, and the coverage page marks them re-check pending.\n\n", len(lines))
	b.WriteString("| Verdict | Scenario | What moved |\n|---|---|---|\n")
	for _, line := range lines {
		fmt.Fprintf(&b, "| `%s/%s` | `%s` | %s |\n", line.Tree, line.Folder, line.Scenario, strings.Join(line.Why, ", "))
	}
	b.WriteString("\nThis reports and never blocks. A paid run of these cases (`make e2e-llm`) " +
		"stamps them again; list them in the PR body if you do not run it.\n")
	return b.String()
}

func emit(text string) {
	if _, err := os.Stdout.WriteString(text); err != nil {
		fail("writing the report: " + err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "mcpe2ecmd: "+message)
	os.Exit(1)
}
