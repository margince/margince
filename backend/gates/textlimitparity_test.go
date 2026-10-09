// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// A text cap a module applies is a number the contract also publishes, and the
// generated server enforces no string length. So each cap is a hand-typed
// mirror of `api/crm.yaml`, and this gate fails the day one of them drifts:
// raise the contract alone and the server refuses what the schema calls valid,
// raise the constant alone and a client truncates to a bound nobody applies.
//
// The list below is the subject, not a census of every maxLength. A cap joins
// it when a module copies the number instead of reading it.

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type textLimit struct {
	file     string
	constant string
	// path walks the parsed contract to the schema node carrying maxLength.
	path []string
}

func schemaPath(parts ...string) []string { return parts }

var textLimits = []textLimit{
	{"internal/compose/bulkvalidate.go", "maxBulkTaskSubject",
		schemaPath("components", "schemas", "BulkTask", "properties", "subject")},
	{"internal/compose/bulkvalidate.go", "maxBulkNote",
		schemaPath("components", "schemas", "BulkChangeExecuteRequest", "properties", "note")},
	{"internal/compose/bulkvalidate.go", "maxBulkNote",
		schemaPath("components", "schemas", "BulkChangePreviewRequest", "properties", "note")},
	{"internal/modules/activities/maildraft.go", "maxDraftSubject",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "subject")},
	{"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "to", "items")},
	{"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "cc", "items")},
	{"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "bcc", "items")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusName", corpusBody("post", "/knowledge/corpora", "name")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusTopic", corpusBody("post", "/knowledge/corpora", "topic_statement")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusDescription", corpusBody("post", "/knowledge/corpora", "description")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusName", corpusBody("patch", "/knowledge/corpora/{id}", "name")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusTopic", corpusBody("patch", "/knowledge/corpora/{id}", "topic_statement")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusDescription", corpusBody("patch", "/knowledge/corpora/{id}", "description")},
}

func corpusBody(method, route, property string) []string {
	return []string{"paths", route, method, "requestBody", "content", "application/json", "schema", "properties", property}
}

func TestEachMirroredTextCapIsTheContractsMaxLength(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	for _, limit := range textLimits {
		published := publishedMaxLength(t, doc, limit.path)
		if applied := appliedIntConst(t, limit.file, limit.constant); applied != published {
			t.Errorf("%s applies %d and api/crm.yaml publishes maxLength %d at %v — one refuses what the other calls valid",
				limit.constant, applied, published, limit.path)
		}
	}
}

// publishedMaxLength walks the contract to a schema node and answers its
// maxLength, failing when the node moved or carries none: a gate whose subject
// vanished must say so rather than pass over nothing.
func publishedMaxLength(t *testing.T, doc map[string]any, path []string) int {
	t.Helper()
	var node any = doc
	for _, key := range path {
		next, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("api/crm.yaml has no %v — the gate's subject moved; repoint it", path)
		}
		if node, ok = next[key]; !ok {
			t.Fatalf("api/crm.yaml has no %q on the way to %v — the gate's subject moved; repoint it", key, path)
		}
	}
	schema, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("api/crm.yaml %v is not a schema node", path)
	}
	length, ok := schema["maxLength"].(int)
	if !ok || length <= 0 {
		t.Fatalf("api/crm.yaml %v publishes no positive maxLength — retire the cap from this gate or restore the bound", path)
	}
	return length
}

// appliedIntConst reads a module's integer constant from source. The constants
// stay unexported: exporting one to suit a gate would widen the surface to make
// the check convenient.
func appliedIntConst(t *testing.T, file, name string) int {
	t.Helper()
	parsed, err := gatekit.ParseFile(file, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name {
					continue
				}
				literal, ok := value.Values[i].(*ast.BasicLit)
				if !ok || literal.Kind != token.INT {
					t.Fatalf("%s in %s is not an integer literal — this gate reads only that shape", name, file)
				}
				text := types.ExprString(literal)
				n, err := strconv.Atoi(text)
				if err != nil {
					t.Fatalf("%s = %q: %v", name, text, err)
				}
				return n
			}
		}
	}
	t.Fatalf("%s declares no %s — the cap the contract publishes is applied by nothing", file, name)
	return 0
}
