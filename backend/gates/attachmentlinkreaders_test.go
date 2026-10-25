// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// A document link row has no bytes and an empty storage_key. Every byte reader
// refuses it through bytes_withheld, which attachment_link_shape forces on.
// So a statement that selects storage_key from the attachment table must also
// test bytes_withheld, or it hands an empty key to the object store.
//
// It reads every string literal in non-test Go under internal/ and extensions/
// as SQL. It cannot see a statement built from fragments in separate literals.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// selectToAttachment spans from a `SELECT` to the first `FROM attachment` after it.
// The span is an over-approximation when one literal holds several statements,
// which can only make the gate stricter.
var selectToAttachment = regexp.MustCompile(`(?is)\bSELECT\b.*?\bFROM\s+attachment\b`)

func selectsAttachmentKey(sql string) bool {
	for _, span := range selectToAttachment.FindAllString(sql, -1) {
		if strings.Contains(span, "storage_key") {
			return true
		}
	}
	return false
}

// minAttachmentKeyReaders is how many such statements the tree held when this
// gate was written: OpenAttachment, the stored-file strip, the vCard import and
// the erasure purge. A smaller count means the pattern stopped seeing a reader.
const minAttachmentKeyReaders = 4

func TestEveryAttachmentKeyReaderTestsBytesWithheld(t *testing.T) {
	t.Parallel()
	var seen int
	for _, root := range []string{"internal", "../extensions"} {
		for path, src := range goSourcesUnder(t, root) {
			file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			for _, decl := range file.Decls {
				owner := declName(decl)
				ast.Inspect(decl, func(node ast.Node) bool {
					literal, ok := node.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						return true
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil || !selectsAttachmentKey(text) {
						return true
					}
					seen++
					if !strings.Contains(text, "bytes_withheld") {
						t.Errorf("%s (in %s) selects an attachment's storage_key without testing "+
							"bytes_withheld. A document link row has no bytes; add NOT bytes_withheld "+
							"(or read the flag) so the row is refused before the object store is asked.",
							path, owner)
					}
					return true
				})
			}
		}
	}
	if seen < minAttachmentKeyReaders {
		t.Errorf("found %d attachment key readers, want at least %d; the pattern no longer sees the tree",
			seen, minAttachmentKeyReaders)
	}
}
