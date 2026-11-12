// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && rolesweep

package toolrole

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/toolroles"
)

// TestTheToolRoleMatrixIsPublished measures, for every seeded role, which tools
// a passport on that role is refused, and holds docs/reference/mcp-tool-roles/,
// one file per verb, to the answer. The roles come from the database and the tools from the registry,
// so neither is a second list. A cell holds what the gate says, not what the seed
// document claims.
func TestTheToolRoleMatrixIsPublished(t *testing.T) {
	recorded := newToolCheckRecorder()
	d := toolRoleApp(t, "tool-role-matrix", compose.WithToolChecksRecordedForTest(recorded.note))
	calls := toolRoleCalls(compose.NewRegistry(nil, compose.SendPath{}).Specs(), routeShapes())
	if len(calls) == 0 {
		t.Fatal("the registry serves no tool, so this measured nothing")
	}
	holdEveryFixtureNamesACall(t, calls)
	roles := apptest.SeededRoles(t, d.E)
	if len(roles) == 0 {
		t.Fatal("the database seeds no role, so this measured nothing")
	}

	verdicts := map[string]toolRoleVerdicts{}
	measured := newToolChecksMeasure()
	elsewhere := measuredElsewhere(t, roles)
	for verb, row := range elsewhere {
		measured.control[verb] = toolroles.Checks{Verb: verb, Checks: row.Checks}
	}
	for _, call := range calls {
		if row, ok := elsewhere[call.verb]; ok {
			verdicts[call.row] = row.toolRoleVerdicts
		}
	}
	seats := roleSeats(t, d.E, roles)
	swept := make([][]sweptCall, len(seats))
	eachSeat(t, "roles", seats, func(t *testing.T, i int, seat roleSeat) {
		swept[i] = sweepSeat(t, seat, recorded, calls, elsewhere)
	})
	if t.Failed() {
		return
	}
	// Merged in the roles' order, so the files read the same however the
	// passes interleaved.
	for i, seat := range seats {
		for _, c := range swept[i] {
			if c.ran {
				measured.add(c.call.verb, seat.role.Key == "admin", c.run, c.got)
			}
			row := verdicts[c.call.row]
			if len(c.releaseRefused) > 0 {
				row.Refused = append(row.Refused, seat.role.Key)
			} else {
				row.record(seat.role.Key, c.got, c.run.Refused)
			}
			verdicts[c.call.row] = row
		}
	}
	measureEveryKind(t, seats, recorded, measured, kindVariants(t, withoutVerbs(calls, elsewhere), seedableKinds(t, d.E)), verdicts)
	colleague := crossOwnerPass(t, d.E, seats, calls)
	if t.Failed() {
		return
	}

	holdTheMeasurementHasTeeth(t, roles, len(calls), verdicts)
	doc, open := toolRolesDocument(roles, calls, verdicts, colleague)
	if len(open) > 0 {
		t.Errorf("%d cells reached no verdict. Each must run, or be refused by a gate:\n%s", len(open), strings.Join(open, "\n"))
	}
	rows, partial := measured.rows()
	holdTheToolChecksTable(t, provenBy(rows, roles, calls, verdicts), partial)
	files, err := doc.published()
	if err != nil {
		t.Fatal(err)
	}
	holdTheFolder(t, toolRolesDir, files, "UPDATE_MCP_TOOL_ROLES=1 make test-roles, then go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity")
}

// A passport is offered a tool or action when, and only when, the gate admits
// its user to it. The roles are an operator's own. One is a copy of rep
// that may create tags and may not update deals. One lacks two checks only
// some paths of a tool ask: a company create, and the installation settings
// read. The last member holds rep and ops, whose grants merge to the widest.
func TestToolsListAgreesWithTheGateForAnOperatorsRoles(t *testing.T) {
	recorded := newToolCheckRecorder()
	d := toolRoleApp(t, "tool-role-listing", compose.WithToolChecksRecordedForTest(recorded.note))
	eachSeat(t, "listing", nil, nil, operatorListings(t, d, recorded)...)
}
