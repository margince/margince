// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// census reads the generators and their outputs from the tree. Each kind of
// generator must be there: a census that finds none of a kind has stopped
// reading the place that kind lives.
func census(root string) (plan, error) {
	listed, err := gitOut(root, nil, "ls-files", "-z")
	if err != nil {
		return plan{}, err
	}
	tracked := strings.Split(strings.TrimSuffix(listed, "\x00"), "\x00")
	p := plan{verified: map[string]string{}}
	if p.outputs, err = generatedOutputs(root, tracked); err != nil {
		return plan{}, err
	}
	if len(p.outputs) == 0 {
		return plan{}, errors.New("no tracked file says a generator wrote it, so the output census read nothing")
	}
	makefile, err := os.ReadFile(filepath.Join(root, driftMakefile)) // #nosec G304 -- a fixed path under the repository root
	if err != nil {
		return plan{}, err
	}
	regen, targets, err := makeGenerator(string(makefile))
	if err != nil {
		return plan{}, err
	}
	p.generators = append(p.generators, regen)
	if p.named, err = namingTools(root, recipeTools(string(makefile), targets)); err != nil {
		return plan{}, err
	}
	if err := p.addGoTests(root, tracked); err != nil {
		return plan{}, err
	}
	frontend, err := frontendGenerators(root)
	if err != nil {
		return plan{}, err
	}
	if _, err := exec.LookPath("pnpm"); err != nil {
		p.skipped = frontend
	} else {
		p.generators = append(p.generators, frontend...)
	}
	return p, nil
}

// addGoTests adds the golden tests a plain build compiles, and marks as unrun
// the outputs whose header names a test behind a build tag.
func (p *plan) addGoTests(root string, tracked []string) error {
	found, err := goGenerators(root, tracked)
	if err != nil {
		return err
	}
	taggedTests := map[string]bool{}
	for _, g := range found {
		if g.Tags != "" {
			p.tagged = append(p.tagged, g)
			for _, test := range g.Tests {
				taggedTests[test] = true
			}
			continue
		}
		command, err := goTestGenerator(root, g)
		if err != nil {
			return err
		}
		p.generators = append(p.generators, command)
	}
	if len(p.generators) == 1 {
		return errors.New("no test reads an -update… flag or UPDATE_… variable, so the golden-file census read nothing")
	}
	p.unrun = map[string]bool{}
	for _, output := range p.outputs {
		byTagged, err := writtenByOneOf(root, output, taggedTests)
		if err != nil {
			return err
		}
		if byTagged {
			p.unrun[output] = true
		}
	}
	return nil
}

// stamped is a time no generator writes at, so a file still carrying it after
// a pass is one no generator wrote.
var stamped = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// stamps holds each output's own modification time, given back at the end.
type stamps struct {
	root     string
	original map[string]time.Time
}

func stampOutputs(root string, outputs []string) (stamps, error) {
	s := stamps{root: root, original: map[string]time.Time{}}
	for _, output := range outputs {
		path := filepath.Join(root, output)
		info, err := os.Stat(path)
		if err != nil {
			return s, fmt.Errorf("stamping %s: %w", output, err)
		}
		s.original[output] = info.ModTime()
		if err := os.Chtimes(path, stamped, stamped); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (s stamps) rewritten(output string) bool {
	info, err := os.Stat(filepath.Join(s.root, output))
	return err == nil && !info.ModTime().Equal(stamped)
}

// restore gives each output its own time back, so Go's test cache, which keys
// on modification times, still holds.
func (s stamps) restore() error {
	var failed []error
	for output, original := range s.original {
		if err := os.Chtimes(filepath.Join(s.root, output), original, original); err != nil {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}
