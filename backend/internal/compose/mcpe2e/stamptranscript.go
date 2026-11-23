// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// transcriptFile holds every parser that turns a driver's stream into the
// transcript the grader reads.
const transcriptFile = "e2e/llm/transcript.py"

// routeParsers maps a verdict's driver to the name prefixes of the transcript
// functions only that driver's stream passes through. The claude CLI writes its
// own stream, so it has none. A fix to one driver's parser then moves that
// driver's verdicts and no other's.
var routeParsers = map[string][]string{
	"claude:cli": nil,
	"gemini:cli": {"_agy_", "from_agy"},
	"gpt:cli":    {"_codex_", "from_codex"},
}

// splitTranscript separates the transcript source into what every driver's
// transcript passes through and each driver's own parser. A top-level def or
// class belongs to the driver whose prefix its name carries. Anything else at
// the left margin is shared, so a constant after a parser is not its own.
func splitTranscript(src string) (shared string, byDriver map[string]string) {
	lines := map[string][]string{}
	owner := ""
	for line := range strings.Lines(src) {
		if name, declares := declaredName(line); declares {
			owner = ownerOf(name)
		} else if line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '\n' && line[0] != '\r' && !strings.ContainsRune(")]}#", rune(line[0])) {
			owner = ""
		}
		lines[owner] = append(lines[owner], line)
	}
	byDriver = map[string]string{}
	for driver := range routeParsers {
		byDriver[driver] = strings.Join(lines[driver], "")
	}
	return strings.Join(lines[""], ""), byDriver
}

// declaredName answers the name a top-level def or class line declares.
func declaredName(line string) (string, bool) {
	for _, keyword := range []string{"async def ", "def ", "class "} {
		if rest, found := strings.CutPrefix(line, keyword); found {
			end := strings.IndexAny(rest, "(: \n")
			if end < 0 {
				end = len(rest)
			}
			return rest[:end], true
		}
	}
	return "", false
}

// ownerOf names the driver whose parser a function name belongs to, or "".
func ownerOf(name string) string {
	for driver, prefixes := range routeParsers {
		if slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			return driver
		}
	}
	return ""
}

// digestHarness is the digest of the files that grade a run. The transcript's
// shared part stands for the transcript, and each driver's parser digest comes
// beside it.
func digestHarness(root string) (harness string, parsers map[string]string, err error) {
	body, err := os.ReadFile(filepath.Join(root, transcriptFile)) // #nosec G304 -- a fixed lane file under the root
	if err != nil {
		return "", nil, err
	}
	shared, byDriver := splitTranscript(string(body))
	harness, err = digestFilesWith(root, harnessFiles, map[string]string{transcriptFile: shared})
	if err != nil {
		return "", nil, err
	}
	parsers = map[string]string{}
	for driver, source := range byDriver {
		if len(routeParsers[driver]) > 0 {
			parsers[driver] = digest([]byte(source))
		}
	}
	return harness, parsers, nil
}
