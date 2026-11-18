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
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/messagingpolicy"
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

// unitsFrom answers the extension folders to compile. With -root every unit
// that has a policy folder or a generated file is one, so a generated file
// whose policy was deleted is found too.
func unitsFrom(root, dir string) ([]string, error) {
	if (root == "") == (dir == "") {
		return nil, errors.New("gen-messaging-rules: pass exactly one of -root and -dir")
	}
	if dir != "" {
		return []string{dir}, nil
	}
	policies, err := filepath.Glob(filepath.Join(root, "extensions", "*", "policy", messagingpolicy.IndexFile))
	if err != nil {
		return nil, err
	}
	generated, err := filepath.Glob(filepath.Join(root, "extensions", "*", messagingpolicy.GeneratedFile))
	if err != nil {
		return nil, err
	}
	var units []string
	for _, path := range policies {
		units = append(units, filepath.Dir(filepath.Dir(path)))
	}
	for _, path := range generated {
		if !slices.Contains(units, filepath.Dir(path)) {
			units = append(units, filepath.Dir(path))
		}
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("gen-messaging-rules: no extension under %s has a policy folder; check -root", root)
	}
	return units, nil
}

// generate compiles one unit's policy and writes, or with check compares, the
// generated file.
func generate(unit string, check bool) error {
	target := filepath.Join(unit, messagingpolicy.GeneratedFile)
	policy, err := messagingpolicy.Compile(os.DirFS(filepath.Join(unit, "policy")))
	if err != nil {
		return err
	}
	pkg, err := packageName(unit)
	if err != nil {
		return err
	}
	src, err := messagingpolicy.Render(pkg, policy.Rules)
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

// packageName reads the package clause of the unit's hand-written Go.
func packageName(unit string) (string, error) {
	files, err := filepath.Glob(filepath.Join(unit, "*.go"))
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_gen.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly)
		if err != nil {
			return "", err
		}
		return parsed.Name.Name, nil
	}
	return "", errors.New("the unit has no hand-written Go file to name its package")
}
