// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bufio"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// seatForeignKey is a head-catalog line declaring a column that names a seat.
var seatForeignKey = regexp.MustCompile(`FOREIGN KEY \(([a-z_]+)\) REFERENCES app_user\(id\)`)

// seatColumnsFromSchema reads every column that references app_user from the
// catalog the migrations are checked against. A new seat column joins the
// census without anybody listing it.
func seatColumnsFromSchema(t *testing.T) []string {
	t.Helper()
	file, err := os.Open("../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("opening the head catalog: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("closing the head catalog: %v", err)
		}
	}()
	seen := map[string]bool{}
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		if match := seatForeignKey.FindStringSubmatch(lines.Text()); match != nil {
			seen[match[1]] = true
		}
	}
	if err := lines.Err(); err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	if len(seen) < 10 || !seen["owner_id"] || !seen["host_user_id"] {
		t.Fatalf("the head catalog yielded %d seat columns, without owner_id or host_user_id: the census is short", len(seen))
	}
	return slices.Sorted(maps.Keys(seen))
}

// unpolicedSeatColumns names every dimension or filter of these specs that
// reads a seat and that namesSeat does not recognise. A seat is read directly
// or through a join that selects one.
func unpolicedSeatColumns(specs map[string]reportSpec, seats []string) []string {
	mentions := regexp.MustCompile(`(^|[^a-z_])(` + strings.Join(seats, "|") + `)([^a-z_]|$)`)
	joined := regexp.MustCompile(`^([a-z_]+)\.([a-z_]+)$`)
	readsSeat := func(spec reportSpec, expr string) bool {
		if mentions.MatchString(withoutActivityPrivacyGuard(expr)) {
			return true
		}
		match := joined.FindStringSubmatch(expr)
		if match == nil {
			return false
		}
		return slices.ContainsFunc(spec.joins, func(join string) bool {
			return strings.Contains(join, ") "+match[1]+" ON") && strings.Contains(join, " AS "+match[2]) && mentions.MatchString(withoutActivityPrivacyGuard(join))
		})
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(specs)) {
		spec := specs[name]
		for _, field := range slices.Sorted(maps.Keys(spec.dimensions)) {
			if readsSeat(spec, spec.dimensions[field]) && !namesSeat(spec, field) {
				out = append(out, name+" dimension "+field)
			}
		}
		for _, field := range slices.Sorted(maps.Keys(spec.filters)) {
			if readsSeat(spec, spec.filters[field]) && !namesSeat(spec, field) {
				out = append(out, name+" filter "+field)
			}
		}
	}
	return out
}

func withoutActivityPrivacyGuard(expression string) string {
	aliases := regexp.MustCompile(`\b([a-z_]+)\.audience\b`)
	for _, match := range aliases.FindAllStringSubmatch(expression, -1) {
		expression = strings.ReplaceAll(expression, auth.WorkspaceAudienceSQL(match[1]), "TRUE")
	}
	return expression
}

func TestMailReadabilityDoesNotCountAsAColleaguesPerformance(t *testing.T) {
	spec := reportSpec{filters: map[string]string{"readable": auth.WorkspaceAudienceSQL("mail")}}
	if got := unpolicedSeatColumns(map[string]reportSpec{"example": spec}, []string{"user_id", "owner_id"}); len(got) != 0 {
		t.Fatalf("privacy join was counted as performance: %v", got)
	}
	spec.filters["readable"] += " AND d.owner_id IS NOT NULL"
	if got := unpolicedSeatColumns(map[string]reportSpec{"example": spec}, []string{"user_id", "owner_id"}); len(got) != 1 {
		t.Fatalf("privacy join hid a real performance column: %v", got)
	}
}

func TestEveryReportColumnNamingASeatAnswersToTheSwitch(t *testing.T) {
	seats := seatColumnsFromSchema(t)
	if missing := unpolicedSeatColumns(everyReportSpec(t), seats); len(missing) > 0 {
		t.Fatalf("these report columns read a seat but the individual-performance switch does not know them; add their expression to seatColumns:\n%s",
			strings.Join(missing, "\n"))
	}
}

// A column the switch does not know must fail the census, read directly or
// through a joined subquery, or the gate above proves nothing.
func TestTheSeatCensusFailsAColumnTheSwitchDoesNotKnow(t *testing.T) {
	seats := seatColumnsFromSchema(t)
	specs := map[string]reportSpec{
		"direct": {entity: datasource.EntityActivity, dimensions: map[string]string{"assignee": "t.assignee_id"}},
		"joined": {
			entity:  datasource.EntityActivity,
			joins:   []string{"LEFT JOIN LATERAL (SELECT p.user_id AS id FROM activity_participant p) seat ON true"},
			filters: map[string]string{"seat": "seat.id"},
		},
		"no seat": {entity: datasource.EntityActivity, dimensions: map[string]string{"kind": colKind}},
	}
	got := unpolicedSeatColumns(specs, seats)
	want := []string{"direct dimension assignee", "joined filter seat"}
	if !slices.Equal(got, want) {
		t.Fatalf("census found %q, want %q", got, want)
	}
}
