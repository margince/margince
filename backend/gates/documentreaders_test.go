// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Every list of attachments in internal/modules/activities answers "which files
// does this record hold" through activities.documentMembership. So a mail's file
// shows on every record the mail is linked to, and no reader keeps a stamp of its own.
//
// What it sees: every function in that package that names attachmentColumns
// and spells a LIMIT, which is a list read. It must call documentMembership
// (directly or through dealDocumentMembership) or be named below.
//
// What it cannot see: a list built from fragments that never name
// attachmentColumns.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gatekit:fixture the functions that list attachments without being a record's
// Documents list, and why
//
// notADocumentList names the functions that read attachments with a LIMIT and
// are not a record's Documents list.
var notADocumentList = map[string]string{
	"PreviewFileCarryTx": "reads one record's own uploads for a merge preview; a mail's file moves with its mail, not with the record",
}

func TestEveryAttachmentListReadUsesTheDocumentMembership(t *testing.T) {
	t.Parallel()
	var members []string
	for path, src := range goSourcesUnder(t, filepath.Join("internal", "modules", "activities")) {
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var namesColumns, limits, usesMembership bool
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.Ident:
					namesColumns = namesColumns || n.Name == "attachmentColumns"
					usesMembership = usesMembership || n.Name == "documentMembership" || n.Name == "dealDocumentMembership"
				case *ast.BasicLit:
					if text, err := strconv.Unquote(n.Value); err == nil && n.Kind == token.STRING {
						limits = limits || strings.Contains(text, "LIMIT")
					}
				}
				return true
			})
			if usesMembership {
				members = append(members, fn.Name.Name)
			}
			if !namesColumns || !limits || usesMembership {
				continue
			}
			if _, excepted := notADocumentList[fn.Name.Name]; !excepted {
				t.Errorf("%s (%s) lists attachments without documentMembership. "+
					"Build its WHERE from documentMembership so the mail files of linked records show, "+
					"or name it in notADocumentList with the reason.", fn.Name.Name, path)
			}
		}
	}
	// A short census: a rename or move would leave the loop above checking nothing.
	if len(members) < 3 {
		t.Errorf("found %d functions using documentMembership (%v), want the contact, company and deal readers",
			len(members), members)
	}
}
