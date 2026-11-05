// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type toolRolesDoc struct {
	Roles []string `json:"roles"`
	Rows  []struct {
		Tool    string   `json:"tool"`
		Action  string   `json:"action"`
		Verb    string   `json:"verb"`
		Allowed []string `json:"allowed_for"`
		Refused []string `json:"refused_for"`
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
		every := slices.Concat(row.Allowed, row.Refused)
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
			t.Errorf("%s is served and docs/reference/mcp-tool-roles.json has no row for it. Run: UPDATE_MCP_TOOL_ROLES=1 make test-roles", row)
		}
	}
	for row := range published {
		if !served[row] {
			t.Errorf("docs/reference/mcp-tool-roles.json has a row for %s, which is not served", row)
		}
	}
}

// readToolChecks is the table the binary embeds, which tools/list narrows by.
func readToolChecks(t *testing.T) agents.ToolChecksFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "modules", "agents", "toolchecks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table agents.ToolChecksFile
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	return table
}

// everyMeasuredGrant holds the admin role key, every grant the table names and
// a row scope every scope check admits. A listing under it is narrowed by
// nothing the table measured.
func everyMeasuredGrant(t *testing.T) principal.Permissions {
	t.Helper()
	perms := principal.Permissions{RoleKeys: []string{"admin"}, Objects: map[string]principal.ObjectGrant{}}
	all := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	var scopeChecks []auth.Check
	for _, row := range readToolChecks(t).Verbs {
		for _, name := range row.Checks {
			check, err := auth.ParseCheck(name)
			switch {
			case err != nil:
				t.Fatal(err)
			case check.RowScope != "" || check.NarrowerThan != "":
				scopeChecks = append(scopeChecks, check)
			case check.Object != "" && !strings.HasPrefix(name, "*"):
				perms.Objects[check.Object] = all
			}
		}
	}
	for _, scope := range []principal.RowScope{principal.RowScopeAll, principal.RowScopeTeam, principal.RowScopeOwn} {
		perms.RowScope = scope
		holder := principal.Principal{Type: principal.PrincipalAgent, Permissions: perms}
		if !slices.ContainsFunc(scopeChecks, func(c auth.Check) bool { return !c.HeldBy(holder) }) {
			return perms
		}
	}
	t.Fatal("no row scope admits every scope check the table names")
	return perms
}
