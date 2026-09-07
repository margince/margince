// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A compiled binary is never tracked.
//
// CLAUDE.md puts them with the build caches: "Regenerable, machine-local, never
// tracked." One reached main anyway — a 19 MB arm64 Mach-O beside its own
// source, committed on a pull request about dropping a database column, and
// caught weeks later by somebody merging main into an unrelated branch and
// reading the created-files list (#2129).
//
// It is worth a gate rather than a deletion because the cost does not end when
// the file does. Removing it in a later commit does not remove it from history:
// every clone downloads those megabytes forever, which is why the rule is
// "never tracked" rather than "clean it up later". And a stale build sitting
// next to its source is the shape of bug where somebody runs the binary instead
// of `go run` and debugs behaviour the code no longer has.
//
// Matched on MAGIC NUMBERS rather than on a mode bit or an extension. The mode
// says a file is executable and ninety tracked scripts are; the extension says
// nothing at all, since the binary that got in had none. What a linker writes
// at the head of its output is the fact this is about, and it is the same fact
// `file` reports.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executableMagic is the first few bytes of a compiled artifact, one entry per
// format a contributor's toolchain can produce here: Linux and the CI runners,
// both macOS architectures and a universal binary, Windows, and WebAssembly.
//
// A universal (fat) Mach-O is listed separately because it is not a Mach-O
// header with a different byte — it is its own container format, and a matcher
// that knew only the thin headers would pass the one artifact most likely to be
// committed by somebody building for two architectures at once.
// gatekit:fixture the magic numbers a linker writes, each mapped to the format name a failure reports
var executableMagic = map[string]string{
	"\x7fELF":          "ELF (Linux)",
	"\xfe\xed\xfa\xce": "Mach-O 32-bit",
	"\xfe\xed\xfa\xcf": "Mach-O 64-bit",
	"\xce\xfa\xed\xfe": "Mach-O 32-bit, byte-swapped",
	"\xcf\xfa\xed\xfe": "Mach-O 64-bit, byte-swapped",
	"\xca\xfe\xba\xbe": "Mach-O universal",
	"MZ":               "PE (Windows)",
	"\x00asm":          "WebAssembly",
}

func TestNoCompiledBinaryIsTracked(t *testing.T) {
	t.Parallel()
	files := trackedFiles(t)
	// A census that read no files would report the tree clean in the same words
	// as a tree with nothing in it, and this one shells out to git.
	if len(files) < 1000 {
		t.Fatalf("read %d tracked file(s) and expected at least 1000 — this census is no longer "+
			"reading the index rather than the tree having emptied", len(files))
	}

	for _, file := range files {
		// A symlink's content is its target's path, which can begin with
		// anything; stat-ing through it would judge a file outside the index.
		if file.symlink {
			continue
		}
		head, err := os.ReadFile(filepath.Join(repoRoot, file.path))
		if err != nil {
			// A tracked path that is not on disk is another gate's finding, and
			// reporting it here would send the reader looking for a binary.
			continue
		}
		if len(head) > 8 {
			head = head[:8]
		}
		for magic, format := range executableMagic {
			if !strings.HasPrefix(string(head), magic) {
				continue
			}
			t.Errorf("%s is a tracked %s executable. Compiled binaries are regenerable, "+
				"machine-local and never tracked (CLAUDE.md), and the cost does not end when the "+
				"file does: removing it in a later commit leaves it in history, so every clone "+
				"downloads it forever.\n"+
				"  git rm --cached the path and add it to .gitignore.",
				file.path, format)
			break
		}
	}
}
