// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"os"
	"path/filepath"
	"regexp"
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
// transcript passes through and each driver's own parser. A top-level def,
// class or constant belongs to the driver whose prefix its name carries.
// Anything else at the left margin is shared. Comments, blank lines and
// decorators belong to the declaration below them, so a driver's comment moves
// that driver's verdicts only.
func splitTranscript(src string) (shared string, byDriver map[string]string) {
	lines := map[string][]string{}
	var leading []string
	owner := ""
	inString := false
	place := func(to, line string) {
		lines[to] = append(lines[to], leading...)
		leading = nil
		lines[to] = append(lines[to], line)
		owner = to
	}
	for line := range strings.Lines(src) {
		switch {
		case inString:
			lines[owner] = append(lines[owner], line)
		case isMarginLead(line):
			leading = append(leading, line)
		case line[0] == ' ' || line[0] == '\t' || strings.ContainsRune(")]}", rune(line[0])):
			place(owner, line)
		default:
			if name, declares := declaredName(line); declares {
				place(ownerOf(name), line)
			} else {
				place("", line)
			}
		}
		if strings.Count(line, `"""`)%2 == 1 || strings.Count(line, "'''")%2 == 1 {
			inString = !inString
		}
	}
	lines[owner] = append(lines[owner], leading...)
	byDriver = map[string]string{}
	for driver := range routeParsers {
		byDriver[driver] = strings.Join(lines[driver], "")
	}
	return strings.Join(lines[""], ""), byDriver
}

// isMarginLead says a line waits for the declaration below it: blank, a
// comment or a decorator.
func isMarginLead(line string) bool {
	return strings.TrimSpace(line) == "" || line[0] == '#' || line[0] == '@'
}

var assignedName = regexp.MustCompile(`^([A-Za-z_]\w*)\s*(?::[^=]+)?=[^=]`)

// declaredName answers the name a top-level def, class or assignment line
// declares.
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
	if match := assignedName.FindStringSubmatch(line); match != nil {
		return match[1], true
	}
	return "", false
}

// ownerOf names the driver whose parser a function name belongs to, or "".
func ownerOf(name string) string {
	lower := strings.ToLower(name)
	for driver, prefixes := range routeParsers {
		if slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(lower, prefix) }) {
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
			parsers[driver] = Digest([]byte(source))
		}
	}
	return harness, parsers, nil
}
