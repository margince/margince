// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestEveryMoveBuilderRegistersItsActionAndCurrentValidator(t *testing.T) {
	constants := map[string]string{}
	sources := moveProductionSources(t)
	for _, source := range sources {
		ast.Inspect(source, func(node ast.Node) bool {
			value, ok := node.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				return true
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			constants[value.Names[0].Name] = text
			return true
		})
	}
	built := map[moveRule]int{}
	types := moveTypeDeclarations(sources)
	for _, source := range sources {
		scanMoveBuilders(t.Errorf, source, constants, built, types)
	}
	if len(built) != len(moveRules) {
		t.Fatalf("%d production builders and %d registered validators", len(built), len(moveRules))
	}
	actions := map[string]bool{ActionNone: true}
	for rule, definition := range moveRules {
		if built[rule] != 1 || definition.validate == nil || definition.action == "" {
			t.Errorf("%s needs one actual builder and a complete action/validator pair", rule)
		}
		if definition.validate(crmcontracts.DealStatusCardMove{}, crmcontracts.DealStatusCardMove{}) {
			t.Errorf("%s accepts advice without a current basis", rule)
		}
		actions[definition.action] = true
	}
	for name, value := range constants {
		if strings.HasPrefix(name, "Action") && !actions[value] {
			t.Errorf("%s has no registered rule and validator", name)
		}
	}
}

func scanMoveBuilders(report func(string, ...any), source *ast.File, constants map[string]string, built map[moveRule]int, types map[string]ast.Expr) {
	parents := moveNodeParents(source)
	for _, decl := range source.Decls {
		function := "package initialization"
		var body ast.Node = decl
		if fn, ok := decl.(*ast.FuncDecl); ok {
			function, body = fn.Name.Name, fn.Body
		}
		ast.Inspect(body, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				scanMoveCall(report, call, function, constants, built)
			}
			if assignment, ok := node.(*ast.AssignStmt); ok {
				for _, target := range assignment.Lhs {
					if field, ok := target.(*ast.SelectorExpr); ok && field.Sel.Name == "Action" {
						report("%s changes an action outside the rule registry", function)
					}
				}
			}
			checkMoveLiteral(report, node, function, parents, types)
			return true
		})
	}
}

func scanMoveCall(report func(string, ...any), call *ast.CallExpr, function string, constants map[string]string, built map[moveRule]int) {
	callee, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}
	switch callee.Name {
	case "move":
		if function != "ruleMove" {
			report("%s bypasses the registered move builder", function)
		}
	case "ruleMove":
		if len(call.Args) == 0 {
			report("%s builds a rule without an identity", function)
			return
		}
		id, ok := call.Args[0].(*ast.Ident)
		if !ok || constants[id.Name] == "" {
			report("%s builds a rule the census cannot identify", function)
			return
		}
		built[moveRule(constants[id.Name])]++
	}
}

func checkMoveLiteral(report func(string, ...any), node ast.Node, function string, parents map[ast.Node]ast.Node, types map[string]ast.Expr) {
	literal, ok := node.(*ast.CompositeLit)
	if !ok || function == "move" {
		return
	}
	kind := resolveMoveType(moveLiteralType(literal, parents, types), types)
	named, ok := kind.(*ast.SelectorExpr)
	if !ok || named.Sel.Name != "DealStatusCardMove" {
		return
	}
	action, ok := reflect.TypeFor[crmcontracts.DealStatusCardMove]().FieldByName("Action")
	if !ok {
		report("the contract's move lost its action field")
		return
	}
	for index, field := range literal.Elts {
		value := field
		if entry, keyed := field.(*ast.KeyValueExpr); keyed {
			key, ok := entry.Key.(*ast.Ident)
			if !ok || key.Name != "Action" {
				continue
			}
			value = entry.Value
		} else if index != action.Index[0] {
			continue
		}
		name, ok := value.(*ast.Ident)
		if !ok || name.Name != "ActionNone" {
			report("%s builds an actionable move outside the rule registry", function)
		}
	}
}

func moveTypeDeclarations(sources []*ast.File) map[string]ast.Expr {
	types := map[string]ast.Expr{}
	for _, source := range sources {
		ast.Inspect(source, func(node ast.Node) bool {
			if declaration, ok := node.(*ast.TypeSpec); ok {
				types[declaration.Name.Name] = declaration.Type
			}
			return true
		})
	}
	return types
}

func resolveMoveType(kind ast.Expr, types map[string]ast.Expr) ast.Expr {
	visited := map[ast.Expr]bool{}
	for kind != nil {
		if visited[kind] {
			return nil
		}
		visited[kind] = true
		switch value := kind.(type) {
		case *ast.Ident:
			next, ok := types[value.Name]
			if !ok {
				return kind
			}
			kind = next
		case *ast.StarExpr:
			kind = value.X
		case *ast.ParenExpr:
			kind = value.X
		default:
			return kind
		}
	}
	return nil
}

func moveLiteralType(literal *ast.CompositeLit, parents map[ast.Node]ast.Node, types map[string]ast.Expr) ast.Expr {
	if literal.Type != nil {
		return literal.Type
	}
	parent := parents[literal]
	if entry, ok := parent.(*ast.KeyValueExpr); ok {
		parent = parents[entry]
	}
	outer, ok := parent.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	switch container := resolveMoveType(moveLiteralType(outer, parents, types), types).(type) {
	case *ast.ArrayType:
		return container.Elt
	case *ast.MapType:
		return container.Value
	default:
		return nil
	}
}

func moveNodeParents(source *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(source, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func TestTheMoveBuilderCensusRejectsUnregisteredConstruction(t *testing.T) {
	for _, body := range []string{
		`func bad() { move(ActionDraftEmail, "", nil) }`,
		`func bad() { ruleMove(unknownRule, "", nil) }`,
		`var bad = crmcontracts.DealStatusCardMove{Action: ActionDraftEmail}`,
		`func bad() { var m crmcontracts.DealStatusCardMove; m.Action = ActionDraftEmail }`,
		`var bad=[]crmcontracts.DealStatusCardMove{{Action:ActionDraftEmail}}`,
		`var bad=map[string]crmcontracts.DealStatusCardMove{"one":{Action:ActionDraftEmail}}`,
		`type Alias=crmcontracts.DealStatusCardMove; var bad=Alias{Action:ActionDraftEmail}`,
		`type Alias=crmcontracts.DealStatusCardMove; var bad=[]*Alias{{Action:ActionDraftEmail}}`,
		positionalMoveFixture(t),
	} {
		source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package dealstatus;"+body, 0)
		if err != nil {
			t.Fatal(err)
		}
		var issues []string
		scanMoveBuilders(func(format string, args ...any) { issues = append(issues, fmt.Sprintf(format, args...)) }, source, nil, map[moveRule]int{}, moveTypeDeclarations([]*ast.File{source}))
		if len(issues) == 0 {
			t.Errorf("the census accepted %s", body)
		}
	}
	source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package dealstatus
 func first() { ruleMove(ruleAcceptRequest, "", nil) }
 var another = func() { ruleMove(ruleAcceptRequest, "", nil) }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	built := map[moveRule]int{}
	scanMoveBuilders(t.Errorf, source, map[string]string{"ruleAcceptRequest": string(ruleAcceptRequest)}, built, moveTypeDeclarations([]*ast.File{source}))
	if built[ruleAcceptRequest] != 2 {
		t.Fatal("a new builder reusing the existing verb escaped the registry census")
	}
}

func positionalMoveFixture(t *testing.T) string {
	t.Helper()
	kind := reflect.TypeFor[crmcontracts.DealStatusCardMove]()
	action, ok := kind.FieldByName("Action")
	if !ok {
		t.Fatal("the contract lost its action field")
	}
	fields := make([]string, kind.NumField())
	for index := range fields {
		fields[index] = "nil"
	}
	fields[action.Index[0]] = "ActionDraftEmail"
	return "var bad=crmcontracts.DealStatusCardMove{" + strings.Join(fields, ",") + "}"
}
