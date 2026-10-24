// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The mail system's own sender names (mailer-daemon, postmaster) are spelled in
// platform/mailidentity and nowhere else. The sender rule is read by capture, by the import write door and by the repair
// of rows stored before the door refused them; a second spelling is a second
// answer, and the two drift until a bounce counts as a reply on one door only.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const deliverySenderOwner = "internal/platform/mailidentity/"

// deliverySenderWaiver is the one file that holds a delivery name as a literal
// for a different question: postmaster there is a role-mailbox word for sorting
// shared mailboxes, not a sender rule.
const deliverySenderWaiver = "internal/platform/mailrole/vocabulary.go"

// foldedDeliveryNames are the spellings the owner folds a local part to.
var foldedDeliveryNames = map[string]bool{"mailerdaemon": true, "postmaster": true}

func foldSeparators(s string) string {
	return strings.NewReplacer(".", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// deliveryNameLiterals returns the string literals in a file that spell a
// delivery-system name once folded.
func deliveryNameLiterals(file *ast.File) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err == nil && foldedDeliveryNames[foldSeparators(value)] {
			found = append(found, value)
		}
		return true
	})
	return found
}

func TestDeliverySystemNamesAreSpelledOnlyInTheSenderRuleOwner(t *testing.T) {
	t.Parallel()
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(path)
		if strings.HasSuffix(slash, "_test.go") {
			continue
		}
		judged++
		if strings.Contains(slash, deliverySenderOwner) {
			continue
		}
		if strings.HasSuffix(slash, deliverySenderWaiver) {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, name := range deliveryNameLiterals(file) {
			findings = append(findings, slash+": "+strconv.Quote(name))
		}
	}
	if judged < 500 {
		t.Fatalf("only %d hand-written files were judged, want at least 500; the census walked too little of the tree", judged)
	}
	if len(findings) > 0 {
		t.Errorf("these files spell a delivery-system sender name outside platform/mailidentity:\n  %s\n\n"+
			"Call mailidentity.IsDeliverySystemSender instead of naming the sender.", strings.Join(findings, "\n  "))
	}
}

func TestTheDeliveryNameCensusSeesEverySpelling(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		name string
		src  string
		want int
	}{
		{"a plain name", `var x = "postmaster"`, 1},
		{"a dashed, upper-case name", `var x = "Mailer-Daemon"`, 1},
		{"an underscored name", "var x = `mailer_daemon`", 1},
		{"a name in a switch case", `func f(s string) { switch s { case "mailerdaemon": } }`, 1},
		{"a name in a comment", "// postmaster\nvar x = 1", 0},
		{"an unrelated word", `var x = "postmaster-general"`, 0},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "probe.go", "package p\n"+probe.src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s: %v", probe.name, err)
		}
		if got := len(deliveryNameLiterals(file)); got != probe.want {
			t.Errorf("%s: found %d literal(s), want %d", probe.name, got, probe.want)
		}
	}
}
