// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every read of an RBAC object's table asks that object's gate, or sits in a
// verdict that says why it need not.
//
// One row per table. Each row's gate, verdicts and floor live in the file named
// in its register field, beside the reasons a reader of that table needs. The
// masked-amount census judges its sites per declaration and shares the judgement
// and the sweep below.

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// objectTableCensus is one row: the gate a read must carry, the verdicts that
// excuse it, and the floor under the count of gated reads.
type objectTableCensus struct {
	gate     objectGate
	verdicts []namedVerdict
	floor    int
	// register is the file holding the row's gate, verdicts and floor, and
	// sweep is its sweep of those verdicts.
	register string
	sweep    func(t *testing.T)
	// reads, gateNoun, discloses and admission are the failure's words.
	reads, gateNoun, discloses, admission string
}

var objectTableCensuses = []objectTableCensus{
	{
		gate: contactGate, verdicts: contactVerdicts, floor: wantMinimumGatedContactSites,
		register: "contactreaders_test.go", sweep: sweepContactVerdicts, reads: "the contact table", gateNoun: "the contact object gate",
		discloses: "a named human being: their name, title, address and employer, which is what the contact grant governs",
		admission: `auth.Require(ctx, "contact", …), or auth.ReadGranted where the read must degrade rather than refuse`,
	},
	{
		gate: dealGate, verdicts: dealVerdicts, floor: wantMinimumGatedDealSites,
		register: "dealreaders_test.go", sweep: sweepDealVerdicts, reads: "the deal table", gateNoun: "the deal object gate",
		discloses: "the commercial position: the figure, the stage and the close date, which is what the deal grant governs",
		admission: `auth.Require(ctx, "deal", …), or auth.ReadGranted where the read must degrade rather than refuse`,
	},
	{
		gate: relationshipGate, verdicts: edgeVerdicts, floor: wantMinimumGatedSites,
		register: "edgereaders_test.go", sweep: sweepEdgeVerdicts, reads: "the relationship table", gateNoun: "the edge gate",
		discloses: "an edge's endpoints AS A PAIR, which relationship.read governs and the endpoints' own grants do not cover",
		admission: "auth.EdgeReadScope",
	},
	{
		gate: companyGate, verdicts: companyVerdicts, floor: wantMinimumGatedCompanySites,
		register: "companyreaders_test.go", sweep: sweepCompanyVerdicts, reads: "the company table", gateNoun: "the company object gate",
		discloses: "who the installation sells to, which is what the company grant governs",
		admission: `auth.Require(ctx, "company", …), or auth.ReadGranted where the read must degrade rather than refuse`,
	},
	{
		gate: leadGate, verdicts: leadVerdicts, floor: wantMinimumGatedLeadSites,
		register: "leadreaders_test.go", sweep: sweepLeadVerdicts, reads: "the lead table", gateNoun: "the lead object gate",
		discloses: "somebody who has not agreed to be a customer, which is what the lead grant governs",
		admission: `auth.Require(ctx, "lead", …)`,
	},
}

func TestEveryReaderOfAnObjectTableCarriesItsGateOrAVerdict(t *testing.T) {
	t.Parallel()
	for _, census := range objectTableCensuses {
		t.Run(census.gate.object, func(t *testing.T) {
			t.Parallel()
			findings, satisfied := census.judgeFiles(t, census.scope().Files(t))
			for _, finding := range findings {
				t.Error(finding)
			}
			census.assertFloor(t, satisfied)
			census.sweep(t)
		})
	}
}

// scope sweeps internal/ for the non-test, non-generated files that read the
// row's table by name, proven against the rest of the module.
func (c objectTableCensus) scope() gatekit.Scope {
	return gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(filePath string, file *ast.File) bool {
			return gatekit.FileReadsTable(filePath, file, c.gate.literal)
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
}

// judgeFiles judges every read site in files: its own body first, then the
// helpers it calls, and never its own name. A by-name lookup would let a gated
// Store.X vouch for an ungated Handlers.X.
func (c objectTableCensus) judgeFiles(t *testing.T, files []gatekit.ParsedFile) (findings []string, satisfied int) {
	t.Helper()
	walk := c.gate.walkPackages(t, files)
	consts := constantTable{}
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, read := range c.gate.readSites(parsed, consts.of(t, pkg)) {
			carries := walk.carries(t, c.gate, parsed, read, consts.of(t, pkg))
			finding, gated := c.judgeSite(t, read.subject(parsed.Path), carries, read.sql)
			if gated {
				satisfied++
			}
			if finding != "" {
				findings = append(findings, finding)
			}
		}
	}
	return findings, satisfied
}

// judgeSite returns the finding for one read and whether it counts as gated. A
// gated read that also holds a verdict fails, because a verdict left behind
// keeps the census from ever counting down.
func (c objectTableCensus) judgeSite(t *testing.T, subject string, carriesGate bool, sql string) (string, bool) {
	t.Helper()
	verdict := verdictIn(t, subject, c.verdicts)
	switch {
	case carriesGate && verdict != "":
		return fmt.Sprintf("%s carries %s AND a %s verdict: remove the verdict, it now describes code "+
			"that is gated", subject, c.gateNoun, verdict), false
	case carriesGate:
		return "", true
	case verdict == "":
		return fmt.Sprintf("%s reads %s without %s and without a verdict.\n"+
			"  The read discloses %s.\n"+
			"  Either ask %s, or declare it in one of the verdicts in %s (%s) with the reason it "+
			"needs no gate.\n"+
			"  Left alone it is indistinguishable from a read nobody considered.\n"+
			"  The read: %s", subject, c.reads, c.gateNoun, c.discloses, c.admission, c.register,
			c.verdictNames(), sql), false
	}
	return "", false
}

func (c objectTableCensus) verdictNames() string {
	names := make([]string, 0, len(c.verdicts))
	for _, verdict := range c.verdicts {
		names = append(names, verdict.name)
	}
	return strings.Join(names, ", ")
}

// assertFloor fails a count of gated reads under the row's floor. An extractor
// that stopped recognising this tree's SQL finds no sites, which reads the same
// as a clean tree.
func (c objectTableCensus) assertFloor(t *testing.T, satisfied int) {
	t.Helper()
	if satisfied < c.floor {
		t.Errorf("only %d reads of %s carry %s, want at least %d: a literal extractor that stopped "+
			"recognising this tree's SQL would report exactly this", satisfied, c.reads, c.gateNoun, c.floor)
	}
	counts := []string{fmt.Sprintf("%d gated", satisfied)}
	for _, verdict := range c.verdicts {
		counts = append(counts, fmt.Sprintf("%d %s", len(verdict.waivers.Subjects()), verdict.name))
	}
	t.Logf("reads of %s: %s", c.reads, strings.Join(counts, ", "))
}

// TestEachObjectTableCensusRefusesAnUngatedRead plants, per row, a read of the
// row's table behind each spelling of its gate and one read behind none. Only
// the last may be a finding.
func TestEachObjectTableCensusRefusesAnUngatedRead(t *testing.T) {
	t.Parallel()
	for _, census := range objectTableCensuses {
		t.Run(census.gate.object, func(t *testing.T) {
			t.Parallel()
			read := `	tx.Query(ctx, "SELECT x.id FROM ` + census.gate.object + ` x")` + "\n"
			var source strings.Builder
			source.WriteString("package fixture\n\nfunc ungated(ctx context.Context, tx pgx.Tx) {\n" + read + "}\n\n" +
				"func required(ctx context.Context, tx pgx.Tx) {\n" +
				"\tif auth.Require(ctx, \"" + census.gate.object + "\", auth.ActionRead) != nil {\n\t\treturn\n\t}\n" +
				read + "}\n")
			for i, seed := range census.gate.gateSeeds {
				fmt.Fprintf(&source, "\nfunc seeded%d(ctx context.Context, tx pgx.Tx) {\n\tauth.%s(ctx)\n%s}\n", i, seed, read)
			}
			fixture := filepath.Join(t.TempDir(), "planted.go")
			if err := os.WriteFile(fixture, []byte(source.String()), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			file, err := gatekit.ParseFile(fixture, 0)
			if err != nil {
				t.Fatalf("parsing the fixture: %v", err)
			}
			findings, satisfied := census.judgeFiles(t, []gatekit.ParsedFile{{Path: filepath.ToSlash(fixture), File: file}})
			if len(findings) != 1 || !strings.Contains(findings[0], ":ungated reads ") {
				t.Errorf("the %s census found %q, want one finding naming ungated", census.gate.object, findings)
			}
			if want := 1 + len(census.gate.gateSeeds); satisfied != want {
				t.Errorf("the %s census counted %d gated reads, want %d: Require and every seed in %v",
					census.gate.object, satisfied, want, census.gate.gateSeeds)
			}
		})
	}
}
