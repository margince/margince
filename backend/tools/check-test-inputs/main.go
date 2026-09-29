// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-test-inputs reads the test logs scripts/testlog-exec.sh
// collected and fails on any package that read a file outside its own module
// without reading that file's tree digest.
//
// Go's test cache never rechecks a path outside the test's module, so such a
// read is invisible to it: a replayed pass outlives any change to the file.
// scripts/ci-stable-mtimes.sh exports one digest per top-level entry of the
// repository, and gatekit.DeclareInputs reads the one a path belongs to — the
// cache keys on every variable a test reads. The census runs against the
// test's real behaviour rather than its source, because a path built at run
// time is exactly what a source scan cannot see.
//
// The variable names come from the script (`--names`), the one place that
// exports them; gatekit serves tests only, so this tool cannot import its copy,
// and a gate holds the two to the same names.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// minimumPackages is the census's floor: fewer logs than this means the run
// that produced them lost most of the tree, and a small corpus reports clean.
const minimumPackages = 100

func main() {
	logs := flag.String("logs", "", "the directory scripts/testlog-exec.sh wrote to")
	namesFile := flag.String("names", "", "the output of scripts/ci-stable-mtimes.sh --names")
	flag.Parse()
	names, err := readNames(*namesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-test-inputs: %v\n", err)
		os.Exit(2)
	}
	findings, read, err := census(*logs, names)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-test-inputs: %v\n", err)
		os.Exit(2)
	}
	if read < minimumPackages {
		fmt.Fprintf(os.Stderr, "check-test-inputs: read %d package logs from %s, too few to be the tree — "+
			"the run that wrote them lost most of it\n", read, *logs)
		os.Exit(2)
	}
	for _, finding := range findings {
		fmt.Println(finding)
	}
	if len(findings) > 0 {
		fmt.Printf("\ncheck-test-inputs: %d read(s) outside a module that Go's test cache cannot see. Pass "+
			"each path to gatekit.DeclareInputs in that package's tests, so a replayed pass keys on "+
			"the tree it read.\n", len(findings))
		os.Exit(1)
	}
	fmt.Printf("OK: check-test-inputs — %d package logs, every read outside a module declared\n", read)
}

// readNames parses `ci-stable-mtimes.sh --names`: one top-level entry and its
// digest variable per line.
func readNames(path string) (map[string]string, error) {
	body, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("reading the digest names: %w", err)
	}
	names := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		top, variable, found := strings.Cut(line, "\t")
		if !found {
			return nil, fmt.Errorf("%s holds %q, not an entry and its variable", path, line)
		}
		names[top] = variable
	}
	return names, nil
}

// census judges every log in dir and reports the undeclared reads, sorted, and
// how many package logs it read.
func census(dir string, names map[string]string) ([]string, int, error) {
	// Rooted at the log directory, so a name read from it cannot reach outside.
	logs := os.DirFS(dir)
	entries, err := fs.ReadDir(logs, ".")
	if err != nil {
		return nil, 0, fmt.Errorf("reading the test logs: %w", err)
	}
	var findings []string
	read := 0
	for _, entry := range entries {
		id, isLog := strings.CutSuffix(entry.Name(), ".log")
		if !isLog {
			continue
		}
		pkgDir, err := fs.ReadFile(logs, id+".dir")
		if err != nil {
			return nil, 0, fmt.Errorf("finding the directory %s ran in: %w", entry.Name(), err)
		}
		log, err := logs.Open(entry.Name())
		if err != nil {
			return nil, 0, fmt.Errorf("opening %s: %w", entry.Name(), err)
		}
		undeclared, err := undeclaredReads(log, strings.TrimSpace(string(pkgDir)), names)
		closeErr := log.Close()
		if err != nil {
			return nil, 0, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		if closeErr != nil {
			return nil, 0, fmt.Errorf("closing %s: %w", entry.Name(), closeErr)
		}
		findings = append(findings, undeclared...)
		read++
	}
	sort.Strings(findings)
	return findings, read, nil
}

// undeclaredReads judges one package's test log: every file or directory it
// opened or stated inside the repository but outside its module must be
// matched by a read of that path's tree digest variable.
func undeclaredReads(log io.Reader, pkgDir string, names map[string]string) ([]string, error) {
	module, repo := enclosing(pkgDir, "go.mod"), enclosing(pkgDir, ".git")
	if module == "" || repo == "" {
		return nil, fmt.Errorf("%s lies in no module or no repository", pkgDir)
	}
	cwd := pkgDir
	read := map[string]bool{}
	outside := map[string]string{}
	scanner := bufio.NewScanner(log)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		op, arg, _ := strings.Cut(scanner.Text(), " ")
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		switch op {
		case "getenv":
			read[arg] = true
		case "chdir":
			cwd = path
		case "open", "stat":
			rel, inRepo := relativeWithin(repo, path)
			if _, inModule := relativeWithin(module, path); inModule || !inRepo || rel == "." {
				continue
			}
			top, _, _ := strings.Cut(rel, "/")
			// git's own bookkeeping, not tracked content: locating the
			// repository stats it, and no digest describes it.
			if top == ".git" {
				continue
			}
			outside[rel] = top
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	pkg, _ := relativeWithin(repo, pkgDir)
	var findings []string
	for path, top := range outside {
		variable, described := names[top]
		switch {
		case !described:
			findings = append(findings, fmt.Sprintf("%s: reads %s, in %s, which no tree digest describes — "+
				"it is not tracked, so declare the tracked source it is built from", pkg, path, top))
		case !read[variable]:
			findings = append(findings, fmt.Sprintf("%s: reads %s and never %s", pkg, path, variable))
		}
	}
	return findings, nil
}

// relativeWithin answers path relative to root, slash-separated, and whether
// it lies in root at all.
func relativeWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	return rel, rel != ".." && !strings.HasPrefix(rel, "../")
}

// enclosing is the nearest directory at or above dir holding name, asked of
// each directory as a rooted filesystem so a logged path cannot steer the stat.
func enclosing(dir, name string) string {
	for ; ; dir = filepath.Dir(dir) {
		if _, err := fs.Stat(os.DirFS(dir), name); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}
