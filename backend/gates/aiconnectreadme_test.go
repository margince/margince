// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The AI access page and the skill README both tell a reader how to connect an
// AI tool. The page reads its paths from aiconnect.ts; the README is written by
// hand. This holds the two to one list: every path the page offers has one
// README section linking the same vendor guide, and the README offers none the
// page lacks. Step wording is left to review, because the page is translated.

import (
	"regexp"
	"strings"
	"testing"
)

const (
	aiConnectModule = "../frontend/src/features/automations/aiconnect.ts"
	skillReadme     = "internal/compose/agentbundle/README.md.tmpl"
	enCatalog       = "../frontend/src/i18n/en.ts"
)

var (
	// A line comment, but not the `//` of a URL, which the package's tsComment
	// would cut out of every help link.
	tsLineComment  = regexp.MustCompile(`(?m)(^|[^:])//[^\n]*`)
	tsStringList   = regexp.MustCompile(`"([a-z-]+)"`)
	tsURLConst     = regexp.MustCompile(`const ([A-Z_]+) =\s*"(https://[^"]+)";`)
	tsPathKey      = regexp.MustCompile(`(?m)^  "([a-z]+-[a-z]+)": \{`)
	tsHelp         = regexp.MustCompile(`help:\s*(?:"(https://[^"]+)"|([A-Z_]+))`)
	tsCliID        = regexp.MustCompile(`id: "([a-z-]+)"`)
	readmeMarker   = regexp.MustCompile(`<!-- connect: ([a-z-]+) -->`)
	settingsTabKey = regexp.MustCompile(`"settings\.tab\.agents":\s*"([^"]+)"`)
)

func TestTheAIAccessPageAndTheSkillReadmeOfferTheSameConnections(t *testing.T) {
	t.Parallel()
	module := tsLineComment.ReplaceAllString(readGateFile(t, aiConnectModule), "$1")
	paths := connectPathHelps(t, module)
	readme := readGateFile(t, skillReadme)

	sections := readmeSections(readme)
	for id, help := range paths {
		section, ok := sections[id]
		switch {
		case !ok:
			t.Errorf("aiconnect.ts offers %q, but README.md.tmpl has no <!-- connect: %s --> section", id, id)
		case !strings.Contains(section, help):
			t.Errorf("README.md.tmpl section %q does not link %s, the guide the page links", id, help)
		}
	}
	for id := range sections {
		if _, ok := paths[id]; !ok {
			t.Errorf("README.md.tmpl has a section for %q, which aiconnect.ts does not offer", id)
		}
	}
	if n := len(readmeMarker.FindAllString(readme, -1)); n != len(sections) {
		t.Errorf("README.md.tmpl marks %d sections for %d paths: a path is marked twice", n, len(sections))
	}

	name := settingsTabKey.FindStringSubmatch(readGateFile(t, enCatalog))
	if name == nil {
		t.Fatal("en.ts has no settings.tab.agents value to check the README against")
	}
	if !strings.Contains(readme, "Settings → "+name[1]) {
		t.Errorf("README.md.tmpl never names the page as **Settings → %s**", name[1])
	}
}

// connectPathHelps maps every path id, tool-plan and command-line, to its help
// URL, and fails when the parse finds fewer paths than the module declares.
func connectPathHelps(t *testing.T, module string) map[string]string {
	t.Helper()
	urls := map[string]string{}
	for _, m := range tsURLConst.FindAllStringSubmatch(module, -1) {
		urls[m[1]] = m[2]
	}
	resolve := func(m []string) string {
		if m[1] != "" {
			return m[1]
		}
		return urls[m[2]]
	}

	tools := literalList(t, module, "AI_TOOLS")
	plans := literalList(t, module, "AI_PLANS")
	block := moduleBlock(t, module, "export const CONNECT_PATHS", "\n};")
	keys := tsPathKey.FindAllStringSubmatchIndex(block, -1)
	paths := map[string]string{}
	for i, key := range keys {
		end := len(block)
		if i+1 < len(keys) {
			end = keys[i+1][0]
		}
		id := block[key[2]:key[3]]
		help := tsHelp.FindStringSubmatch(block[key[1]:end])
		if help == nil || resolve(help) == "" {
			t.Fatalf("aiconnect.ts path %q has no help URL this gate can read", id)
		}
		paths[id] = resolve(help)
	}
	for _, tool := range tools {
		for _, plan := range plans {
			if _, ok := paths[tool+"-"+plan]; !ok {
				t.Fatalf("aiconnect.ts declares %s and %s, but no %s-%s path this gate can read", tool, plan, tool, plan)
			}
		}
	}
	if len(paths) != len(tools)*len(plans) {
		t.Fatalf("read %d tool paths, want %d tools × %d plans", len(paths), len(tools), len(plans))
	}

	cli := moduleBlock(t, module, "export const CLI_PATHS", "\n];")
	ids := tsCliID.FindAllStringSubmatchIndex(cli, -1)
	if len(ids) == 0 {
		t.Fatal("aiconnect.ts lists no command-line path this gate can read")
	}
	for i, id := range ids {
		end := len(cli)
		if i+1 < len(ids) {
			end = ids[i+1][0]
		}
		help := tsHelp.FindStringSubmatch(cli[id[1]:end])
		if help == nil || resolve(help) == "" {
			t.Fatalf("aiconnect.ts command-line path %q has no help URL", cli[id[2]:id[3]])
		}
		paths[cli[id[2]:id[3]]] = resolve(help)
	}
	return paths
}

// readmeSections is each marked section's text, up to the next marker or the
// next top-level heading.
func readmeSections(readme string) map[string]string {
	sections := map[string]string{}
	marks := readmeMarker.FindAllStringSubmatchIndex(readme, -1)
	for i, m := range marks {
		end := len(readme)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		}
		body := readme[m[1]:end]
		if cut := strings.Index(body, "\n## "); cut >= 0 {
			body = body[:cut]
		}
		sections[readme[m[2]:m[3]]] = body
	}
	return sections
}

func literalList(t *testing.T, module, name string) []string {
	t.Helper()
	line := moduleBlock(t, module, "export const "+name+" = [", "]")
	var out []string
	for _, m := range tsStringList.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("aiconnect.ts %s lists nothing this gate can read", name)
	}
	return out
}

// moduleBlock is the module text between two markers, failing on a marker the
// module no longer carries rather than reading an empty block as agreement.
func moduleBlock(t *testing.T, module, start, end string) string {
	t.Helper()
	block := between(module, start, end)
	if block == "" {
		t.Fatalf("aiconnect.ts has no %q block ending %q", start, end)
	}
	return block
}
