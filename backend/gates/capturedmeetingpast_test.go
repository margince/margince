// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The mail permission check and consent count a captured meeting only once its
// time has passed and nobody called it off. Both ask relstrength.MeetingInTouchSQL.
//
// Capture's check decides whether a meeting's guest becomes a contact. Consent's
// decides whether writing to that contact is lawful. A booking still in the
// diary is evidence for neither. A module never imports a sibling, so each
// spells its own SQL, and this holds both to the one kernel rule. A status or
// time bound spelled by hand beside it fails here.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const relstrengthPath = "github.com/margince/margince/backend/internal/shared/kernel/relstrength"

// capturedMeetingReaders are the two declarations that judge a captured
// meeting, each by the file and name that hold it.
var capturedMeetingReaders = []struct{ file, name string }{
	{"internal/modules/capture/correspondencegate.go", "metInPersonTx"},
	{"internal/modules/consent/qualifyingground.go", "capturedMeetingSQL"},
}

func TestACapturedMeetingCountsOnlyOnceItIsInTouch(t *testing.T) {
	t.Parallel()
	for _, reader := range capturedMeetingReaders {
		file, err := gatekit.ParseFile(filepath.FromSlash(reader.file), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", reader.file, err)
		}
		decl := declarationNamed(file, reader.name)
		if decl == nil {
			t.Errorf("%s no longer declares %s, so this gate reads nothing there; point it at the "+
				"declaration that judges a captured meeting", reader.file, reader.name)
			continue
		}
		qualifier, _ := gatekit.ImportedAs(file, relstrengthPath)
		if qualifier == "" || !callsSelector(decl, qualifier, "MeetingInTouchSQL") {
			t.Errorf("%s in %s does not ask relstrength.MeetingInTouchSQL, so a meeting still in "+
				"the diary, or one called off, can stand as evidence for the mail permission check "+
				"or consent. Bound it with that rule", reader.name, reader.file)
		}
		if spelled := spellsMeetingBound(decl); spelled != "" {
			t.Errorf("%s in %s spells %q by hand beside relstrength.MeetingInTouchSQL. The two drift; "+
				"leave the status and the time to the shared rule", reader.name, reader.file, spelled)
		}
	}
}

// declarationNamed answers the function or package-level value named name.
func declarationNamed(file *ast.File, name string) ast.Node {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == name {
				return d
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok && len(value.Names) == 1 && value.Names[0].Name == name {
					return value
				}
			}
		}
	}
	return nil
}

// spellsMeetingBound answers the first SQL literal in node that names the
// meeting status or the meeting's time, or "" when none does.
func spellsMeetingBound(node ast.Node) string {
	found := ""
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || found != "" {
			return found == ""
		}
		for _, bound := range []string{"meeting_status", "occurred_at <=", "now() +"} {
			if strings.Contains(lit.Value, bound) {
				found = bound
			}
		}
		return found == ""
	})
	return found
}

// callsSelector reports whether node names qualifier.symbol anywhere inside it.
func callsSelector(node ast.Node, qualifier, symbol string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != symbol {
			return !found
		}
		if ident, isIdent := sel.X.(*ast.Ident); isIdent && ident.Name == qualifier {
			found = true
		}
		return !found
	})
	return found
}
