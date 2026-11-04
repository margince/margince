// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateMCPToolRolesPage = flag.Bool("update-mcp-tool-roles-page", false,
	"rewrite docs/reference/mcp-tool-roles.md from docs/reference/mcp-tool-roles.json")

type toolRolesDoc struct {
	Note    string            `json:"note"`
	Reasons map[string]string `json:"reasons"`
	Roles   []string          `json:"roles"`
	Rows    []struct {
		Tool            string            `json:"tool"`
		Action          string            `json:"action"`
		Allowed         []string          `json:"allowed_for"`
		Refused         []string          `json:"refused_for"`
		Unproven        []string          `json:"unproven_for"`
		UnprovenReasons map[string]string `json:"unproven_reasons"`
		ColleagueRecord map[string]string `json:"colleague_record"`
	} `json:"rows"`
}

func readToolRoles(t *testing.T) toolRolesDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(mcpDocsDir, "mcp-tool-roles.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc toolRolesDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// The integration lane measures the JSON. This lane holds it to the surface, so
// a tool or action that ships without a row fails before any database.
func TestTheToolRoleMatrixCoversEveryServedAction(t *testing.T) {
	doc := readToolRoles(t)
	published := map[string]bool{}
	for _, row := range doc.Rows {
		published[row.Tool+"."+row.Action] = true
		every := slices.Concat(row.Allowed, row.Refused, row.Unproven)
		slices.Sort(every)
		roles := slices.Sorted(slices.Values(doc.Roles))
		if !slices.Equal(every, roles) {
			t.Errorf("%s %s sorts the roles %v; want each of %v exactly once", row.Tool, row.Action, every, roles)
		}
	}
	served := map[string]bool{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if spec.Unlisted {
			continue
		}
		if len(spec.Actions) == 0 {
			served[spec.Name+"."] = true
		}
		for _, action := range spec.Actions {
			served[spec.Name+"."+action.Action] = true
		}
	}
	if len(served) == 0 {
		t.Fatal("the registry serves no tool, so this checked nothing")
	}
	for row := range served {
		if !published[row] {
			t.Errorf("%s is served and docs/reference/mcp-tool-roles.json has no row for it. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished", row)
		}
	}
	for row := range published {
		if !served[row] {
			t.Errorf("docs/reference/mcp-tool-roles.json has a row for %s, which is not served", row)
		}
	}
}

func TestTheToolRolePageIsTheMatrix(t *testing.T) {
	page := filepath.Join(mcpDocsDir, "mcp-tool-roles.md")
	rendered := renderToolRoles(readToolRoles(t))
	if *updateMCPToolRolesPage {
		if err := os.WriteFile(page, rendered, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, rendered) {
		t.Errorf("%s is not its JSON. Run: go test ./internal/compose/ -run TestTheToolRolePageIsTheMatrix -update-mcp-tool-roles-page", page)
	}
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
| ? | Not proven either way. The call stopped first; [why it stopped](#why-a-call-stopped) counts the reasons. |

A cell with counts, such as ` + "`5✓ 2✗ 3?`" + `, covers the actions of one router. Row scope, the seat and the passport's
scopes still apply to each call. A ✓ says the role passed on a record it owns;
[a colleague's record](#a-colleagues-record) shows what the narrower roles get on one they do not own.

Regenerate from the repository root, then from ` + "`backend/`" + `:

    UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished
    go test ./internal/compose/ -run TestTheToolRolePageIsTheMatrix -update-mcp-tool-roles-page

## The matrix

`

func renderToolRoles(doc toolRolesDoc) []byte {
	var b bytes.Buffer
	b.WriteString(toolRolesPageHead)
	b.WriteString("> " + doc.Note + "\n")
	b.WriteString(toolRolesPageMarks)
	b.WriteString("| Tool | " + strings.Join(doc.Roles, " | ") + " | Actions some role is refused |\n")
	b.WriteString("|---|" + strings.Repeat("---|", len(doc.Roles)+1) + "\n")
	var tools []string
	for _, row := range doc.Rows {
		if !slices.Contains(tools, row.Tool) {
			tools = append(tools, row.Tool)
		}
	}
	for _, tool := range tools {
		cells := make([]string, 0, len(doc.Roles))
		for _, role := range doc.Roles {
			cells = append(cells, toolRoleCell(doc, tool, role))
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", tool, strings.Join(cells, " | "), refusedActions(doc, tool))
	}
	renderStopReasons(&b, doc)
	renderColleagueRecord(&b, doc)
	return b.Bytes()
}

// renderStopReasons counts why the unproven cells stopped, for the admin
// control alone and for every role.
func renderStopReasons(b *bytes.Buffer, doc toolRolesDoc) {
	admin, every := map[string]int{}, map[string]int{}
	for _, row := range doc.Rows {
		for role, reason := range row.UnprovenReasons {
			every[reason]++
			if role == "admin" {
				admin[reason]++
			}
		}
	}
	b.WriteString("\n## Why a call stopped\n\nA cell is proven only where the admin passes the same call, so the admin column is the work left.\n\n")
	b.WriteString("| Reason | Meaning | admin | Every role |\n|---|---|---|---|\n")
	for _, reason := range slices.Sorted(maps.Keys(doc.Reasons)) {
		fmt.Fprintf(b, "| `%s` | %s | %d | %d |\n", reason, doc.Reasons[reason], admin[reason], every[reason])
	}
}

// renderColleagueRecord counts, per scoped role, what the calls on a record a
// colleague owns were answered, and names the ones that ran.
func renderColleagueRecord(b *bytes.Buffer, doc toolRolesDoc) {
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
				ran[role] = append(ran[role], "`"+strings.TrimSuffix(row.Tool+"."+row.Action, ".")+"`")
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
}

func toolRoleCell(doc toolRolesDoc, tool, role string) string {
	var allowed, refused, unproven int
	for _, row := range doc.Rows {
		switch {
		case row.Tool != tool:
		case slices.Contains(row.Allowed, role):
			allowed++
		case slices.Contains(row.Refused, role):
			refused++
		case slices.Contains(row.Unproven, role):
			unproven++
		}
	}
	marks := []struct {
		count int
		mark  string
	}{{allowed, "✓"}, {refused, "✗"}, {unproven, "?"}}
	if allowed+refused+unproven == 1 {
		for _, m := range marks {
			if m.count == 1 {
				return m.mark
			}
		}
	}
	var parts []string
	for _, m := range marks {
		if m.count > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", m.count, m.mark))
		}
	}
	return strings.Join(parts, " ")
}

func refusedActions(doc toolRolesDoc, tool string) string {
	var entries []string
	for _, row := range doc.Rows {
		if row.Tool == tool && row.Action != "" && len(row.Refused) > 0 {
			entries = append(entries, fmt.Sprintf("`%s` (%s)", row.Action, strings.Join(row.Refused, ", ")))
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return "<details><summary>" + fmt.Sprint(len(entries)) + "</summary>" + strings.Join(entries, " ") + "</details>"
}
