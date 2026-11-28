// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Field codes travel inside details.errors and have their own server census.
func emittedFieldCodes(t *testing.T) map[string]string {
	t.Helper()
	c := newEmittedCensus(t, readGoUnits(t, "internal"))
	c.fieldCodes = true
	c.readPluralFieldSources()
	for name := range fieldCodeCarriers {
		shape := c.structs[problemWriterDir][name]
		for _, set := range c.carrierLiterals(name) {
			if code, ok := fieldValue(set.lit, shape, "Code"); ok {
				c.read(code, set.unit, set.in)
			}
		}
	}
	for key, calls := range c.calls {
		if key.dir != problemWriterDir || key.name != "Validation" {
			continue
		}
		for _, site := range calls {
			if len(site.call.Args) > 1 {
				c.read(site.call.Args[1], site.unit, site.in)
			}
		}
	}
	for _, unit := range c.units {
		for _, decl := range unit.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "FieldFault" || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				if ret, ok := node.(*ast.ReturnStmt); ok && len(ret.Results) == 3 {
					c.read(ret.Results[1], unit, fn)
				}
				return true
			})
		}
	}
	for _, fault := range c.faults {
		t.Error(fault)
	}
	delete(c.codes, "")
	for code, at := range c.codes {
		if len(code) > httperr.MaxFaultText {
			t.Errorf("%s: field code %q exceeds the wire limit", at, code)
		}
	}
	return c.codes
}

// The enclosing Problem supplies the field code's status and provenance.
func (r *problemFileReader) fieldProblemContext(open int) (statusRead, bool) {
	tr := r.tree
	array := tr.parent[open]
	if array < 0 || !tr.is(array, "[") {
		return statusRead{}, false
	}
	details := tr.parent[array]
	if details < 0 || !tr.is(details, "{") || !r.holdsMember(details, "errors", array) {
		return statusRead{}, false
	}
	problem := tr.parent[details]
	if problem < 0 || !tr.is(problem, "{") || !r.holdsMember(problem, "details", details) {
		return statusRead{}, false
	}
	return r.problemContext(problem, map[int]bool{}), true
}

func (r *problemFileReader) holdsMember(open int, key string, value int) bool {
	for _, entry := range r.tree.entries(open) {
		if entry.key == key && entry.value.from <= value && value < entry.value.to {
			return true
		}
	}
	return false
}

// TestEveryProblemMockFieldCodeIsSentByTheServer checks the emitted Problem contract.
func TestEveryProblemMockFieldCodeIsSentByTheServer(t *testing.T) {
	t.Parallel()
	known := emittedFieldCodes(t)
	if len(known) < 30 {
		t.Fatalf("read only %d field codes; the census lost its callers", len(known))
	}
	mocks, scans := frontendProblemMocks(t)
	admitted := unknownFieldSites(t, scans, known)
	count := 0
	for _, mock := range mocks {
		if !mock.field {
			continue
		}
		count++
		if admitted[mock] {
			continue
		}
		if mock.computed {
			t.Errorf("%s:%d: field code cannot be followed to a literal", mock.path, mock.line)
			continue
		}
		if _, sent := known[mock.code]; !sent {
			t.Errorf("%s:%d: field code %q is sent by no server path", mock.path, mock.line, mock.code)
		}
	}
	if count < 20 {
		t.Fatalf("read only %d field code mocks; the scan lost details.errors", count)
	}
}

func unknownFieldSites(t *testing.T, scans map[string]*problemFileScan, known map[string]string) map[problemMock]bool {
	t.Helper()
	admitted := map[problemMock]bool{}
	for path, scan := range scans {
		for _, marker := range scan.markers {
			_, reason, found := strings.Cut(marker.text, unknownFieldCodeMarker)
			if !found || !marker.line {
				continue
			}
			line := scan.reader.lineOfPos(marker.pos)
			at := fmt.Sprintf("%s:%d", path, line)
			if !mockFile.MatchString(path) || strings.TrimSpace(reason) == "" {
				t.Errorf("%s: %s needs a test fixture and a reason", at, unknownFieldCodeMarker)
				continue
			}
			matched := false
			for _, mock := range scan.mocks {
				if mock.field && !mock.computed && (mock.line == line || mock.line == line+1) {
					if _, sent := known[mock.code]; sent {
						t.Errorf("%s: marked field code %q is emitted by the server", at, mock.code)
					}
					admitted[mock] = true
					matched = true
				}
			}
			if !matched {
				t.Errorf("%s: %s vouches for no field code on its line or the next", at, unknownFieldCodeMarker)
			}
		}
	}
	return admitted
}
