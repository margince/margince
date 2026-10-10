// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The handler of a PATCH or PUT with a nullable body field processes explicit
// nulls, by clearing or refusing the field. A decoded pointer reads a null and "not sent" alike. So the handler reads the
// nulls through httperr.ClearedFields, NullKeys or PresentField, or sits in
// nullIgnoredByHandler. This proves the handler asks, not what it does with the
// answer. A waiver for a handler that now asks fails.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	typedPointers    = "typed pointers: a null is read as absent, so the field cannot be cleared"
	patchUnconfirmed = "the handler reads no explicit null and no test sends one; unconfirmed whether the store honours it"
	putUnconfirmed   = "a replace: the store may take every column from the body; no test sends a null"
)

// nullIgnoredByHandler are the operations whose handler reads no explicit null.
// A null-clears test, or a read of the nulls in the handler, removes the entry.
var nullIgnoredByHandler = gatekit.Waive(map[string]string{
	"UpdateAutomation":                patchUnconfirmed,
	"UpdateContract":                  typedPointers,
	"UpdateDealRoom":                  patchUnconfirmed,
	"UpdateOffer":                     typedPointers,
	"UpdateOfferLineItem":             patchUnconfirmed,
	"UpdateProduct":                   typedPointers,
	"UpdateSignal":                    patchUnconfirmed,
	"EditWeeklyPlanCommitment":        patchUnconfirmed,
	"PutAnchorCompany":                putUnconfirmed,
	"SetConnectorContextTag":          putUnconfirmed,
	"SetConnectorSignatureEnrichment": putUnconfirmed,
	"SetDealRoomExpiry":               putUnconfirmed,
	"SetLeadManualSignal":             putUnconfirmed,
	"SaveMailDraft":                   putUnconfirmed,
	"SaveMyGreetingName":              putUnconfirmed,
	"PutOnboardingState":              putUnconfirmed,
	"SetWeeklyPlanContract":           putUnconfirmed,
})

func TestEveryHandlerOfANullableBodyReadsExplicitNullsSomewhere(t *testing.T) {
	t.Parallel()
	defer nullIgnoredByHandler.AssertAllMatched(t)
	doc := loadContractDocument(t)
	schemas := descendContract(t, doc, "components", "schemas")
	ops := nullableBodyOperations(t, doc, schemas)
	// One subject per spelling of "nullable": a type list, an enum holding
	// null, and the OpenAPI 3.0 flag.
	known := map[string]string{
		"UpdateRelationship": "role", "UpdateRetentionPolicy": "lawful_basis", "EditWeeklyPlanCommitment": "linked_record",
	}
	for handler, field := range known {
		if !slices.ContainsFunc(ops, func(op nullableOperation) bool {
			return op.handler == handler && slices.Contains(op.fields, field)
		}) {
			t.Fatalf("the census found %d operation(s) and not %s with %s; the walk lost its subjects", len(ops), handler, field)
		}
	}
	handlers := handlerCalls(t)
	var missing []string
	for _, op := range ops {
		calls, found := handlers[op.handler]
		if !found {
			t.Errorf("%s %s: no handler method %s was found, so the census cannot judge it", op.method, op.path, op.handler)
			continue
		}
		if readsExplicitNulls(calls) {
			continue
		}
		if nullIgnoredByHandler.Waived(t, op.handler) {
			continue
		}
		missing = append(missing, op.method+" "+op.path+" "+op.handler+" "+strings.Join(op.fields, ","))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d operation(s) type a body field as nullable and never read an explicit null in their handler:\n\t%s\n\n"+
			"Read the nulls with httperr.ClearedFields and clear through storekit.ApplyClears (or refuse a "+
			"field this record cannot clear), or record why the handler is exempt.",
			len(missing), strings.Join(missing, "\n\t"))
	}
}

type nullableOperation struct {
	method, path, handler string
	fields                []string
}

// nullableBodyOperations lists each PATCH and PUT whose JSON body schema holds a
// property that admits null, with the handler method the generated router calls.
func nullableBodyOperations(t *testing.T, doc, schemas map[string]any) []nullableOperation {
	t.Helper()
	var out []nullableOperation
	paths := descendContract(t, doc, "paths")
	for path, item := range paths {
		pathItem, _ := item.(map[string]any)
		for _, method := range []string{"patch", "put"} {
			op, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			schema := bodySchema(op)
			if schema == nil {
				continue
			}
			var fields []string
			for name, prop := range bodyProperties(schemas, schema) {
				if bodyPropertyAdmitsNull(schemas, prop) {
					fields = append(fields, name)
				}
			}
			if len(fields) == 0 {
				continue
			}
			sort.Strings(fields)
			id, _ := op["operationId"].(string)
			if id == "" {
				t.Errorf("%s %s has no operationId", strings.ToUpper(method), path)
				continue
			}
			out = append(out, nullableOperation{
				method: strings.ToUpper(method), path: path,
				handler: strings.ToUpper(id[:1]) + id[1:], fields: fields,
			})
		}
	}
	return out
}

func bodySchema(op map[string]any) map[string]any {
	body, _ := op["requestBody"].(map[string]any)
	content, _ := body["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	return schema
}

func resolveSchema(schemas, node map[string]any) map[string]any {
	for {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node
		}
		next, ok := schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
		if !ok {
			return node
		}
		node = next
	}
}

func bodyProperties(schemas, schema map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	node := resolveSchema(schemas, schema)
	collect := func(from map[string]any) {
		props, _ := from["properties"].(map[string]any)
		for name, prop := range props {
			if p, ok := prop.(map[string]any); ok {
				out[name] = p
			}
		}
	}
	collect(node)
	parts, _ := node["allOf"].([]any)
	for _, part := range parts {
		if p, ok := part.(map[string]any); ok {
			collect(resolveSchema(schemas, p))
		}
	}
	return out
}

func bodyPropertyAdmitsNull(schemas, prop map[string]any) bool {
	node := resolveSchema(schemas, prop)
	if types, ok := node["type"].([]any); ok && slices.Contains(types, any("null")) {
		return true
	}
	if flag, _ := node["nullable"].(bool); flag {
		return true
	}
	enum, _ := node["enum"].([]any)
	return slices.Contains(enum, nil)
}

// handlerCalls maps each method name declared on a type to the names of the
// functions that method calls. A parsed call cannot be met by a comment or a
// string that mentions a reader.
func handlerCalls(t *testing.T) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	err := filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") || strings.Contains(path, "internal/contracts/") {
			return nil
		}
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if out[fn.Name.Name] == nil {
				out[fn.Name.Name] = map[string]bool{}
			}
			maps.Copy(out[fn.Name.Name], calledNames(fn))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	return out
}

// calledNames names every function the body calls, by its last identifier.
func calledNames(fn *ast.FuncDecl) map[string]bool {
	calls := map[string]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.SelectorExpr:
			calls[callee.Sel.Name] = true
		case *ast.Ident:
			calls[callee.Name] = true
		}
		return true
	})
	return calls
}

// readsExplicitNulls is whether the calls include a reader of a body's nulls.
func readsExplicitNulls(calls map[string]bool) bool {
	return calls["ClearedFields"] || calls["PresentField"] || calls["NullKeys"] || calls["NullKeysOf"]
}

// A reader named in a comment or a string is not a read.
func TestAMentionOfAReaderIsNotACall(t *testing.T) {
	t.Parallel()
	const source = `package h

func (h H) InAComment() {
	// TODO: call httperr.ClearedFields(r) here
}

func (h H) InAString() {
	_ = "httperr.PresentField(r, name)"
}

func (h H) Calls() {
	_ = httperr.ClearedFields(r)
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "h.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"InAComment": false, "InAString": false, "Calls": true}
	for _, decl := range file.Decls {
		fn := decl.(*ast.FuncDecl)
		if got := readsExplicitNulls(calledNames(fn)); got != want[fn.Name.Name] {
			t.Errorf("%s: reads = %v, want %v", fn.Name.Name, got, want[fn.Name.Name])
		}
	}
}
