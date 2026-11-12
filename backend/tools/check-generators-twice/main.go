// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-generators-twice runs two passes of every generator whose
// output the tree commits. It fails when the two passes leave the tree
// different in any byte. A drift gate compares one fresh run with the
// committed file, so output that depends on map order passes it for a while.
//
// The generators come from the tree. They are the regeneration the drift gate
// runs, every test that reads an -update… flag or UPDATE_… variable, and every
// gen:* script of the frontend. The outputs come from the tree too: every tracked
// file whose name or header says a generator wrote it. The first pass must
// rewrite each output, so a generator this census cannot see fails the gate.
// A test behind a build tag, such as integration, is listed and not run; so
// is each output whose header names such a test.
//
// The work tree is put back as it was: this gate judges determinism, and the
// drift gates judge whether the committed files are current.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "..", "the repository root")
	list := flag.Bool("list", false, "print the generators and outputs the census finds, and run nothing")
	flag.Parse()
	abs, err := filepath.Abs(*root)
	if err == nil && *list {
		err = printCensus(abs, log.New(os.Stdout, "", 0))
	} else if err == nil {
		err = run(abs, log.New(os.Stdout, "", 0))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-generators-twice: %v\n", err)
		os.Exit(1)
	}
}

// plan is what one census of the tree found.
type plan struct {
	generators []generator
	outputs    []string
	// unrun are the outputs a tagged test writes: listed, never required.
	unrun  map[string]bool
	tagged []goGenerator
	// named maps a path to the tool of the drift regeneration whose source
	// names it, for an output that tool need not rewrite.
	named map[string]string
	// verified are the outputs no pass rewrote and a tool names.
	verified map[string]string
	// skipped are the frontend scripts, when pnpm is absent. The outputs no
	// other generator wrote are then listed as not checked.
	skipped []generator
}

func run(root string, out *log.Logger) error {
	p, err := census(root)
	if err != nil {
		return err
	}
	return twice(root, p, out)
}

// twice runs the plan's generators twice and puts the work tree back.
func twice(root string, p plan, out *log.Logger) (err error) {
	scratch, err := os.MkdirTemp("", "generators-twice-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(scratch)) }()
	tree, err := newWorktree(root, scratch)
	if err != nil {
		return err
	}
	before, err := tree.snapshot()
	if err != nil {
		return err
	}
	stamps, err := stampOutputs(root, p.outputs)
	if err == nil {
		err = judge(p, stamps, tree, out)
	}
	// The bytes go back first. A file with its old time over new bytes of the
	// same size would replay a cached test result that no longer holds.
	if restoreErr := tree.restore(before); restoreErr != nil {
		return errors.Join(err, fmt.Errorf("putting the work tree back: %w", restoreErr))
	}
	return errors.Join(err, stamps.restore())
}

// judge runs both passes and names what disagreed.
func judge(p plan, stamps stamps, tree worktree, out *log.Logger) error {
	first, err := pass(tree.root, p.generators, tree, out, 1)
	if err != nil {
		return err
	}
	var unwritten []string
	for _, output := range p.outputs {
		if p.unrun[output] || stamps.rewritten(output) {
			continue
		}
		if tool, ok := namedBy(output, p.named); ok {
			p.verified[output] = tool
			continue
		}
		unwritten = append(unwritten, output)
	}
	if len(unwritten) > 0 && len(p.skipped) > 0 {
		out.Printf("check-generators-twice: NOT CHECKED, no pnpm on PATH to run the frontend generators:\n  %s\n",
			strings.Join(unwritten, "\n  "))
	} else if len(unwritten) > 0 {
		return fmt.Errorf("no generator this gate runs wrote %d file(s) that say a generator wrote them:\n  %s\n"+
			"Give the generator an -update… flag in a test, a gen:* script in %s, or a place in `make gen`",
			len(unwritten), strings.Join(unwritten, "\n  "), frontendPackage)
	}
	second, err := pass(tree.root, p.generators, tree, out, 2)
	if err != nil {
		return err
	}
	differ, err := tree.changed(first, second)
	if err != nil {
		return err
	}
	if len(differ) > 0 {
		var names []string
		for _, d := range differ {
			names = append(names, d[1])
		}
		return fmt.Errorf("two runs of the same generators wrote different bytes to %d file(s):\n  %s\n"+
			"A generator reads map order, time or something else that moves; make it read a sorted list",
			len(differ), strings.Join(names, "\n  "))
	}
	out.Printf("check-generators-twice: %d generator(s) ran twice and left the same bytes\n", len(p.generators))
	details(p, out)
	return nil
}

// pass runs every generator once and snapshots what they left.
func pass(root string, generators []generator, tree worktree, out *log.Logger, number int) (string, error) {
	for _, g := range generators {
		started := time.Now()
		cmd := exec.Command(g.Argv[0], g.Argv[1:]...) // #nosec G204 -- argv comes from the tree's own Makefile, package.json and tests
		cmd.Dir = filepath.Join(root, g.Dir)
		cmd.Env = append(os.Environ(), g.Env...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("pass %d: %s: %w\n%s", number, g.Name, err, output)
		}
		out.Printf("pass %d: %-90s %6.1fs\n", number, g.Name, time.Since(started).Seconds())
	}
	return tree.snapshot()
}

func printCensus(root string, out *log.Logger) error {
	p, err := census(root)
	if err != nil {
		return err
	}
	for _, g := range p.generators {
		out.Printf("generator: %s\n", g.Name)
	}
	for _, output := range p.outputs {
		out.Printf("output: %s\n", output)
	}
	details(p, out)
	return nil
}

// details names what the gate did not run or did not see rewritten.
func details(p plan, out *log.Logger) {
	for _, g := range p.tagged {
		out.Printf("  not run, needs build tags %s: %s %s\n", g.Tags, g.Dir, strings.Join(g.Tests, " "))
	}
	for _, g := range p.skipped {
		out.Printf("  NOT RUN, no pnpm on PATH: %s\n", g.Name)
	}
	for _, output := range slices.Sorted(maps.Keys(p.verified)) {
		out.Printf("  not rewritten, named by tools/%s: %s\n", p.verified[output], output)
	}
	for _, output := range slices.Sorted(maps.Keys(p.unrun)) {
		out.Printf("  not checked, a tagged test writes it: %s\n", output)
	}
}
