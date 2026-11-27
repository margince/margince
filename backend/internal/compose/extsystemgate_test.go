// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every system read of a unit answers for a seat. The test lists the ports from
// the published source, so a method added later is held to the rule at once.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// seatExempt are the reads published before v1.0.0. They keep their shape, and
// the five without actAs are deprecated for the seat-bound form named here.
// SetAside answers about the member's own choice, so it names that member as
// user. Mail takes actAs already and binds any member who connected the unit,
// under the earlier rule.
// gatekit:fixture the frozen reads, each with the reason it keeps its shape
var seatExempt = map[string]string{
	"Deals.Get": "GetAs", "Deals.Responsible": "ResponsibleAs", "Authority.CanRead": "CanReadAs", "Authority.CanWrite": "CanWriteAs",
	"Directory.Emails": "EmailsAs", "SetAside.Contact": "own choice", "SetAside.Contacts": "own choice", "Activities.Mail": "connected member",
}

// systemReadMethod is one method of a system-read port, as the source declares it.
type systemReadMethod struct {
	port, name string
	hasActAs   bool
}

// systemReadMethods walks every interface in the system-read file.
func systemReadMethods(t *testing.T) []systemReadMethod {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../../pkg/extension/systemread.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []systemReadMethod
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		for _, m := range iface.Methods.List {
			fn, ok := m.Type.(*ast.FuncType)
			if !ok || len(m.Names) == 0 {
				continue
			}
			method := systemReadMethod{port: spec.Name.Name, name: m.Names[0].Name}
			for _, param := range fn.Params.List {
				for _, name := range param.Names {
					method.hasActAs = method.hasActAs || name.Name == "actAs"
				}
			}
			out = append(out, method)
		}
		return true
	})
	return out
}

// implementers maps each port's interface name to the compose type behind it,
// found through the Runtime method that returns the port.
func implementers(t *testing.T, ports map[string]bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	rt := reflect.TypeFor[extension.Runtime]()
	impl := reflect.ValueOf(&callRuntime{})
	for m := range rt.Methods() {
		if m.Type.NumIn() != 0 || m.Type.NumOut() != 1 || !ports[m.Type.Out(0).Name()] {
			continue
		}
		got := impl.MethodByName(m.Name).Call(nil)[0]
		out[m.Type.Out(0).Name()] = got.Elem().Type().Name()
	}
	return out
}

// callsSeatRead reports whether the method named name on recv calls seatRead.
func callsSeatRead(t *testing.T, recv, name string) bool {
	t.Helper()
	files := parsePackageFiles(t, token.NewFileSet(), ".")
	found, calls := false, false
	{
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Name.Name != name || receiverName(fn.Recv.List[0].Type) != recv {
					continue
				}
				found = true
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "seatRead" {
							calls = true
						}
					}
					return true
				})
			}
		}
	}
	if !found {
		t.Errorf("%s.%s has no implementation in this package", recv, name)
	}
	return calls
}

func receiverName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func TestEverySystemReadTakesActAsAndGoesThroughTheSeatGate(t *testing.T) {
	methods := systemReadMethods(t)
	ports := map[string]bool{}
	for _, m := range methods {
		ports[m.port] = true
	}
	if len(methods) == 0 {
		t.Fatal("no system-read port found in pkg/extension/systemread.go")
	}
	impl := implementers(t, ports)
	seen := map[string]bool{}
	for _, m := range methods {
		key := m.port + "." + m.name
		if _, frozen := seatExempt[key]; frozen {
			seen[key] = true
			continue
		}
		if !m.hasActAs {
			t.Errorf("%s takes no actAs parameter. A system read answers for a seat.", key)
		}
		if recv := impl[m.port]; recv == "" {
			t.Errorf("%s: no Runtime method returns the port, so its implementation was not found", key)
		} else if !callsSeatRead(t, recv, m.name) {
			t.Errorf("%s.%s does not call seatRead, the one gate of a seat read", recv, m.name)
		}
	}
	for key := range seatExempt {
		if !seen[key] {
			t.Errorf("%s is listed as exempt but is no method of a system-read port", key)
		}
	}
}
