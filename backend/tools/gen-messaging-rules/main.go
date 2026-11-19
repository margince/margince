// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// gen-messaging-rules compiles each extension's policy/ folder into the
// messagingrules_gen.go beside it: the messaging.Rules the pack declares. With
// -check it writes nothing and exits 1 when a policy does not compile or a
// generated file differs from its sources.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/margince/margince/backend/internal/platform/messagingpolicy"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func main() {
	root := flag.String("root", "", "the repository root: compile every extensions/*/policy")
	dir := flag.String("dir", "", "one extension folder, as go generate passes it")
	check := flag.Bool("check", false, "write nothing; exit 1 when a generated file differs from its policy")
	flag.Parse()
	if err := run(*root, *dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, dir string, check bool) error {
	units, err := unitsFrom(root, dir)
	if err != nil {
		return err
	}
	var failures []error
	for _, unit := range units {
		if err := generate(unit, check); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", unit, err))
		}
	}
	return errors.Join(failures...)
}

// generate compiles one unit's policy and writes, or with check compares, the
// generated file.
func generate(unit string, check bool) error {
	target := filepath.Join(unit, messagingpolicy.GeneratedFile)
	files, err := handWritten(unit)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("the unit has no hand-written Go file to name its package")
	}
	if !generatesRules(files) {
		return fmt.Errorf("no hand-written file carries a //go:generate line running %s; add gen.go", generatorName)
	}
	src, err := compileAndRender(unit, files[0].Name.Name)
	if err != nil {
		return err
	}
	if !check {
		//nolint:gosec // the output sits beside the unit's own source
		return os.WriteFile(target, src, 0o644)
	}
	have, err := os.ReadFile(target) // #nosec G304 -- the unit's own generated file
	if err != nil || !bytes.Equal(have, src) {
		return fmt.Errorf("%s does not match policy/; run `make -C backend gen` or `go generate` in the unit", target)
	}
	return nil
}

// compileAndRender compiles the unit's policy folder in whichever layout it
// has and answers the generated source for package pkg.
func compileAndRender(unit, pkg string) ([]byte, error) {
	compiled, err := messagingpolicy.CompileUnit(os.DirFS(filepath.Join(unit, "policy")))
	if err != nil {
		return nil, err
	}
	if !compiled.PerCountry {
		return messagingpolicy.Render(pkg, compiled.Policies[0].Rules)
	}
	rules := make([]messaging.Rules, 0, len(compiled.Policies))
	for _, policy := range compiled.Policies {
		rules = append(rules, policy.Rules)
	}
	return messagingpolicy.RenderSet(pkg, rules)
}
