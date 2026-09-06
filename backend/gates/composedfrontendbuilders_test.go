// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Four builders compile the composed SPA — the make lane, the release image and
// the desktop bundle on each platform — and every one of them must install the
// composed workspace before it does.
//
// That install is the ONLY thing that gives a unit's frontend layer its own
// dependencies. It stopped being the root workspace's job when a unit's
// frontend/ stopped being a member there, and the image build was not brought
// along: it ran `pnpm build:composed` over a tree where a unit's react,
// @tanstack/react-query and @types/react resolved to nothing, and every unit
// screen failed TS2307. Nothing said so, because no lane builds that image —
// the defect waited for a release.
//
// The corpus is derived from the invocation rather than listed: a file that
// drives the composed build through pnpm is a builder, whatever it is written
// in. A list would have carried exactly the three entries that were right.
//
// The grain is the FILE, not the individual lane, so a Makefile that kept the
// install on one lane and lost it on another still reads clean here. That gap is
// covered elsewhere and not by argument: `make check-fe` runs the make lanes on
// every pull request, so a lane that loses its install goes red the same day.
// Nothing in the merge gate builds the image or the desktop bundles, which is
// precisely where the defect lived and what this gate is for.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// composedBuildInvocation is a builder driving the composed lane: `pnpm
// build:composed`, or the typecheck projects the make lane runs directly.
// frontend/package.json DEFINES that script and does not invoke it, which is
// why the pattern asks for pnpm on the same line.
var composedBuildInvocation = regexp.MustCompile(`pnpm.*(build:composed|tsc -p tsconfig\.composed)`)

// composedWorkspaceDirDecl reads the emitter's own constant, so this gate
// follows the directory if it ever moves instead of holding a second spelling
// of where it is.
var composedWorkspaceDirDecl = regexp.MustCompile(`(?m)^const composedFrontendWorkspaceDir = "([^"]+)"`)

func TestEveryComposedFrontendBuilderInstallsTheWorkspace(t *testing.T) {
	t.Parallel()
	workspace := composedWorkspaceDir(t)

	var builders int
	for _, file := range trackedFiles(t) {
		// This file spells the invocation it looks for, so scanning it would
		// report itself. The exemption is the exact path: a second file of the
		// same basename elsewhere is scanned like everything else.
		if file.symlink || file.path == "backend/gates/composedfrontendbuilders_test.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, file.path))
		if err != nil {
			// Tracked but absent happens mid-rebase and after `git rm`.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("reading %s: %v", file.path, err)
		}
		lines := strings.Split(string(body), "\n")
		if !buildsTheComposedLane(lines) {
			continue
		}
		builders++
		if !namesInCode(lines, workspace) {
			t.Errorf("%s builds the composed SPA without installing %s — a unit's frontend layer gets react, @tanstack/react-query and @types/react from that install and from nowhere else, so every unit screen fails TS2307",
				file.path, workspace)
		}
	}
	if builders == 0 {
		t.Fatal("no file drives the composed build — a sweep that finds no builder reports clean exactly like one that checked every builder, so either the scripts moved or this pattern is stale")
	}
}

// composedWorkspaceDir is the directory the generator writes, read from the
// generator.
func composedWorkspaceDir(t *testing.T) string {
	t.Helper()
	const emitter = "tools/gen-composition/emitfrontend.go"
	body, err := os.ReadFile(emitter)
	if err != nil {
		t.Fatalf("reading %s: %v", emitter, err)
	}
	match := composedWorkspaceDirDecl.FindSubmatch(body)
	if match == nil {
		t.Fatalf("%s no longer declares composedFrontendWorkspaceDir — this gate cannot say what a builder has to install", emitter)
	}
	return string(match[1])
}

func buildsTheComposedLane(lines []string) bool {
	for _, line := range lines {
		if !isComment(line) && composedBuildInvocation.MatchString(line) {
			return true
		}
	}
	return false
}

// namesInCode ignores comments, so a builder cannot satisfy this by explaining
// in prose why it skips the install.
func namesInCode(lines []string, want string) bool {
	for _, line := range lines {
		// PowerShell spells the same path with backslashes.
		if !isComment(line) && strings.Contains(strings.ReplaceAll(line, `\`, "/"), want) {
			return true
		}
	}
	return false
}

// isComment covers the comment markers of every language a builder here is
// written in: make, shell, Dockerfile and PowerShell use `#`, Go uses `//`.
func isComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//")
}
