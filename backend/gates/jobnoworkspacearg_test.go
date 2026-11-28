// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// An installation holds one company, so no job names a workspace. A job args
// type that carried one again would write a key that no reader of river_job
// looks at any more.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// argsTypeFloor guards against a vacuous pass: the walker only reports on the
// types it finds, so one that matched nothing would read green.
const argsTypeFloor = 100

const (
	workspaceArgField = "Workspace"
	workspaceArgKey   = "workspace_id"
)

func TestNoJobArgsCarryAWorkspace(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("internal", "compose")
	byType := methodsByType(t, dir)
	fset, files := parseGoFilesUnder(t, dir)

	checked := 0
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !byType[typeSpec.Name.Name]["Kind"] {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				checked++
				assertNoWorkspaceArg(t, fset, typeSpec.Name.Name, structType)
			}
		}
	}
	if checked < argsTypeFloor {
		t.Fatalf("inspected only %d job args types, expected at least %d: the walker matched nothing and this gate would pass vacuously", checked, argsTypeFloor)
	}
}

// assertNoWorkspaceArg refuses a field called Workspace, a field that ships
// json:"workspace_id", and an embedded type. The gate reads declared fields, so
// it cannot see a key inside an embedded type.
func assertNoWorkspaceArg(t *testing.T, fset *token.FileSet, typeName string, structType *ast.StructType) {
	t.Helper()
	for _, field := range structType.Fields.List {
		pos := fset.Position(field.Pos())
		if len(field.Names) == 0 {
			t.Errorf("%s:%d: %s embeds a type; job args must declare their fields so this gate can read them.", pos.Filename, pos.Line, typeName)
			continue
		}
		if declaresName(field, workspaceArgField) || jsonKey(t, fset, typeName, field) == workspaceArgKey {
			t.Errorf("%s:%d: %s names a workspace; an installation holds one company, so the job needs none.", pos.Filename, pos.Line, typeName)
		}
	}
}

// declaresName reports whether a struct field is declared under the given name.
func declaresName(field *ast.Field, want string) bool {
	for _, name := range field.Names {
		if name.Name == want {
			return true
		}
	}
	return false
}

// jsonKey returns a field's wire name, dropping the `,omitempty`-style options.
// strconv.Unquote handles both a raw-string and a quoted-string tag.
func jsonKey(t *testing.T, fset *token.FileSet, typeName string, field *ast.Field) string {
	t.Helper()
	if field.Tag == nil {
		return ""
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		pos := fset.Position(field.Tag.Pos())
		t.Fatalf("%s:%d: %s's struct tag %s will not unquote: %v", pos.Filename, pos.Line, typeName, field.Tag.Value, err)
	}
	name, _, _ := strings.Cut(reflect.StructTag(raw).Get("json"), ",")
	return name
}
