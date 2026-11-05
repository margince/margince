// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// docs/reference/mcp-tool-roles.md is drawn here, beside the parity catalog,
// because the catalog's places are the page's sections. They live in this
// package's hand-kept table and in a walk only this package runs. So the page
// is rendered from the report itself, and no second copy of the places exists.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

const (
	toolRolesPage   = "../docs/reference/mcp-tool-roles.md"
	toolChecksTable = "internal/modules/agents/toolchecks.json"
	// unplacedGroup holds the calls whose verb serves no one operation, such as
	// a native tool or one that reads any route.
	unplacedGroup = "no_one_operation"
)

// toolRolesFile is the sweep's published matrix, as this page reads it.
type toolRolesFile struct {
	Note    string            `json:"note"`
	Reasons map[string]string `json:"reasons"`
	Roles   []string          `json:"roles"`
	Rows    []toolRoleCell    `json:"rows"`
}

type toolRoleCell struct {
	Tool            string            `json:"tool"`
	Action          string            `json:"action"`
	Verb            string            `json:"verb"`
	Allowed         []string          `json:"allowed_for"`
	Refused         []string          `json:"refused_for"`
	ColleagueRecord map[string]string `json:"colleague_record"`
}

func (c toolRoleCell) call() string { return strings.TrimSuffix(c.Tool+"."+c.Action, ".") }

// toolChecksFile is the checks table the binary embeds.
type toolChecksFile struct {
	Note  string `json:"note"`
	Verbs map[string]struct {
		Checks []string `json:"checks"`
	} `json:"verbs"`
}

// readSweepFile reads one of the files the role sweep writes.
func readSweepFile[T toolRolesFile | toolChecksFile](t *testing.T, path string) T {
	t.Helper()
	var into T
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, &into); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	return into
}

// callPlace is where one swept call sits on the page.
type callPlace struct{ group, area string }

// placesOfVerbs places each verb that serves one operation, and no other, where the
// catalog put that operation.
func placesOfVerbs(report parityReport) map[string]callPlace {
	places := map[string]callPlace{}
	serves := map[string]int{}
	for _, op := range report.Operations {
		if op.Verb == "" {
			continue
		}
		serves[op.Verb]++
		places[op.Verb] = callPlace{op.Group, op.Area}
	}
	for verb, n := range serves {
		if n > 1 {
			delete(places, verb)
		}
	}
	return places
}

// placedSection is one group of the page, with its calls by area in the
// catalog's order.
type placedSection struct {
	key, title string
	areas      []placedArea
}

type placedArea struct {
	title string
	calls []toolRoleCell
}

func placedSections(report parityReport, doc toolRolesFile) []placedSection {
	places := placesOfVerbs(report)
	byPlace := map[callPlace][]toolRoleCell{}
	for _, row := range doc.Rows {
		place, ok := places[row.Verb]
		if !ok {
			place = callPlace{unplacedGroup, unplacedGroup}
		}
		byPlace[place] = append(byPlace[place], row)
	}
	var sections []placedSection
	for _, group := range report.Groups {
		section := placedSection{key: group.Key, title: group.Title}
		for _, area := range group.Areas {
			if calls := byPlace[callPlace{group.Key, area.Key}]; len(calls) > 0 {
				section.areas = append(section.areas, placedArea{area.Title, calls})
			}
		}
		if len(section.areas) > 0 {
			sections = append(sections, section)
		}
	}
	if calls := byPlace[callPlace{unplacedGroup, unplacedGroup}]; len(calls) > 0 {
		sections = append(sections, placedSection{
			key: unplacedGroup, title: "Tools no one operation serves",
			areas: []placedArea{{"", calls}},
		})
	}
	return sections
}

func syncToolRolesPage(t *testing.T, report parityReport) {
	t.Helper()
	doc := readSweepFile[toolRolesFile](t, toolRolesJSON)
	checks := readSweepFile[toolChecksFile](t, toolChecksTable)
	syncMCPParity(t, toolRolesPage, renderToolRolesPage(placedSections(report, doc), doc, checks))
}

const toolRolesPageHead = `# Which role can use which MCP tool

<!-- Generated together with mcp-tool-roles.json; do not edit by hand. -->

An agent has the access of the user it acts for. This page shows what that gives on each role a new
workspace starts with. [rbac-matrix.md](rbac-matrix.md) lists the grants themselves;
[agent-tools.md](agent-tools.md) lists the tools.

Nobody types this page. It is drawn from [mcp-tool-roles.json](mcp-tool-roles.json), and the test that
writes the file fails when the answers change and the file does not. The file's note says how it is
measured:

`

const toolRolesPageMarks = `
| Mark | Meaning |
|---|---|
| ✓ | The call ran for this role. |
| ✗ | A gate refused this role. |

Row scope, the seat and the passport's scopes still apply to each call. A ✓ says the role passed on a
record it owns; [a colleague's record](#a-colleagues-record) shows what the narrower roles get on one
they do not own.

The sections follow the groups and areas of [mcp-parity.md](mcp-parity.md). A call sits where the
catalog puts the one operation its verb serves. A verb that serves none, or several, is under
[Tools no one operation serves](#tools-no-one-operation-serves).

Regenerate from the repository root, then from ` + "`backend/`" + `:

    UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/toolrole RUN=TestTheToolRoleMatrixIsPublished
    go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity

A call that reaches no verdict fails the first command; fix it as
[prove-a-tool-in-the-role-matrix.md](../how-to/prove-a-tool-in-the-role-matrix.md) says.
`

func renderToolRolesPage(sections []placedSection, doc toolRolesFile, checks toolChecksFile) []byte {
	var b bytes.Buffer
	b.WriteString(toolRolesPageHead)
	b.WriteString("> " + doc.Note + "\n")
	b.WriteString(toolRolesPageMarks)
	renderToolRolesSummary(&b, sections, doc.Roles)
	for _, section := range sections {
		fmt.Fprintf(&b, "\n## %s\n", section.title)
		for _, area := range section.areas {
			if area.title != "" {
				fmt.Fprintf(&b, "\n### %s\n", area.title)
			}
			b.WriteString("\n| Call | " + strings.Join(doc.Roles, " | ") + " |\n|---|" + strings.Repeat("---|", len(doc.Roles)) + "\n")
			for _, call := range area.calls {
				cells := make([]string, len(doc.Roles))
				for i, role := range doc.Roles {
					cells[i] = map[bool]string{true: "✓", false: "✗"}[slices.Contains(call.Allowed, role)]
				}
				fmt.Fprintf(&b, "| `%s` | %s |\n", call.call(), strings.Join(cells, " | "))
			}
		}
	}
	renderColleagueRecord(&b, doc)
	renderCallNeeds(&b, sections, checks)
	return b.Bytes()
}

func renderToolRolesSummary(b *bytes.Buffer, sections []placedSection, roles []string) {
	b.WriteString("\n## The groups\n\nEach cell counts the calls in the group that ran for the role, and those a gate refused.\n\n")
	b.WriteString("| Group | Calls | " + strings.Join(roles, " | ") + " |\n|---|---:|" + strings.Repeat("---|", len(roles)) + "\n")
	for _, section := range sections {
		calls := 0
		allowed := map[string]int{}
		for _, area := range section.areas {
			for _, call := range area.calls {
				calls++
				for _, role := range call.Allowed {
					allowed[role]++
				}
			}
		}
		cells := make([]string, len(roles))
		for i, role := range roles {
			cells[i] = fmt.Sprintf("%d✓ %d✗", allowed[role], calls-allowed[role])
		}
		fmt.Fprintf(b, "| [%s](#%s) | %d | %s |\n", section.title, anchorOf(section.title), calls, strings.Join(cells, " | "))
	}
}

// anchorOf is the heading anchor a Markdown renderer gives a title.
func anchorOf(title string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r == ' ':
			out.WriteRune('-')
		case r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
		}
	}
	return out.String()
}

// renderColleagueRecord counts, per scoped role, what the calls on a record a
// colleague owns were answered, and names the ones that ran.
func renderColleagueRecord(b *bytes.Buffer, doc toolRolesFile) {
	counts := map[string]map[string]int{}
	ran := map[string][]string{}
	answers := map[string]bool{}
	for _, row := range doc.Rows {
		for role, answer := range row.ColleagueRecord {
			if counts[role] == nil {
				counts[role] = map[string]int{}
			}
			counts[role][answer]++
			answers[answer] = true
			if answer == "ran" {
				ran[role] = append(ran[role], "`"+row.call()+"`")
			}
		}
	}
	b.WriteString("\n## A colleague's record\n\nEach role below reads fewer rows than all. Every granted seat reads a company, contact, " +
		"deal, lead or project, so a call may run; the test fails when one changes the colleague's record: its version, " +
		"update time, archive or owner.\n\n")
	kinds := slices.Sorted(maps.Keys(answers))
	b.WriteString("| Role | " + strings.Join(kinds, " | ") + " | Calls that ran |\n|---|" + strings.Repeat("---|", len(kinds)+1) + "\n")
	for _, role := range slices.Sorted(maps.Keys(counts)) {
		cells := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			cells = append(cells, fmt.Sprint(counts[role][kind]))
		}
		fmt.Fprintf(b, "| %s | %s | <details><summary>%d</summary>%s</details> |\n", role, strings.Join(cells, " | "), len(ran[role]), strings.Join(ran[role], " "))
	}
	b.WriteString("\n| Answer | Meaning |\n|---|---|\n")
	for _, kind := range kinds {
		if meaning, ok := doc.Reasons[kind]; ok {
			fmt.Fprintf(b, "| `%s` | %s |\n", kind, meaning)
		}
	}
}

// renderCallNeeds lists what each swept verb's calls were asked, which is what
// tools/list holds a passport's user to, in the matrix's sections.
func renderCallNeeds(b *bytes.Buffer, sections []placedSection, checks toolChecksFile) {
	b.WriteString("\n## What each call needs\n\n" +
		"tools/list offers a passport a tool, or a router's action, only when its user holds every check below. " +
		"The checks come from [the tool checks table](../../backend/internal/modules/agents/toolchecks.json), " +
		"which the same test writes:\n\n> " + checks.Note + "\n\n")
	b.WriteString("A check is `object.action`, or `object.a|b` for any one of them. `*.action` is the action on any " +
		"object, `admin` is the admin role key, `scope:team` is a row scope at least that wide, and " +
		"`scope-narrower-than:all` is a row scope short of all.\n")
	for _, section := range sections {
		fmt.Fprintf(b, "\n### %s: checks\n\n| Area | Call | Verb | Checks |\n|---|---|---|---|\n", section.title)
		for _, area := range section.areas {
			for _, call := range area.calls {
				names := make([]string, 0, len(checks.Verbs[call.Verb].Checks))
				for _, name := range checks.Verbs[call.Verb].Checks {
					names = append(names, "`"+name+"`")
				}
				fmt.Fprintf(b, "| %s | `%s` | `%s` | %s |\n", area.title, call.call(), call.Verb, strings.Join(names, " "))
			}
		}
	}
}
