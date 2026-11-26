// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestEveryTextProducerClassifiesItsContent(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	contracts, err := textProducerContracts(root)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	err = gatekit.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		source, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		corpus, err := textProducers(path, source, contracts)
		if err != nil {
			return err
		}
		count += len(corpus)
		for _, producer := range corpus {
			if !producer.classified {
				t.Errorf("%s:%d: classify each product or original component with SystemText", path, producer.line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count < 150 {
		t.Fatalf("producer census read only %d constructors", count)
	}
}

type textProducer struct {
	line       int
	classified bool
}

type producerContracts map[string]map[string]bool

func textProducerContracts(root string) (producerContracts, error) {
	contracts := producerContracts{}
	err := gatekit.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			shape, ok := declaration.Type.(*ast.StructType)
			if !ok {
				return true
			}
			metadata := false
			for _, field := range shape.Fields.List {
				for _, name := range field.Names {
					metadata = metadata || name.Name == "SystemText"
				}
			}
			if metadata {
				if contracts[file.Name.Name] == nil {
					contracts[file.Name.Name] = map[string]bool{}
				}
				contracts[file.Name.Name][declaration.Name.Name] = true
			}
			return true
		})
		return nil
	})
	return contracts, err
}

func textProducers(path string, source []byte, contracts producerContracts) ([]textProducer, error) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, source, 0)
	if err != nil {
		return nil, err
	}
	packages := map[string]string{file.Name.Name: file.Name.Name}
	aliases := map[string]ast.Expr{}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(imported)
		alias := name
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		packages[alias] = name
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if declaration, ok := node.(*ast.TypeSpec); ok && declaration.Assign.IsValid() {
			aliases[declaration.Name.Name] = declaration.Type
		}
		return true
	})
	var producers []textProducer
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			if function.Body != nil {
				producers = append(producers, producersIn(function.Body, positions, file.Name.Name, packages, aliases, contracts)...)
			}
		} else {
			producers = append(producers, producersIn(declaration, positions, file.Name.Name, packages, aliases, contracts)...)
		}
	}
	return producers, nil
}

func producerContract(expression ast.Expr, local string, packages map[string]string, aliases map[string]ast.Expr, contracts producerContracts) bool {
	for range 16 {
		switch kind := expression.(type) {
		case *ast.Ident:
			if alias, ok := aliases[kind.Name]; ok {
				expression = alias
				continue
			}
			return contracts[local][kind.Name]
		case *ast.SelectorExpr:
			if alias, ok := kind.X.(*ast.Ident); ok {
				return contracts[packages[alias.Name]][kind.Sel.Name]
			}
		case *ast.StarExpr:
			expression = kind.X
			continue
		}
		return false
	}
	return false
}

type producerFields struct{ text, metadata bool }

func isProducerTextField(name string) bool {
	return name == "Subject" || name == "Body" || name == "Summary"
}

func producerLiteral(expression ast.Expr) *ast.CompositeLit {
	if pointer, ok := expression.(*ast.UnaryExpr); ok && pointer.Op == token.AND {
		expression = pointer.X
	}
	literal, _ := expression.(*ast.CompositeLit)
	return literal
}

func producersIn(node ast.Node, positions *token.FileSet, local string, packages map[string]string, aliases map[string]ast.Expr, contracts producerContracts) []textProducer {
	bound := map[*ast.CompositeLit]string{}
	assigned := map[string]producerFields{}
	inferred := map[*ast.CompositeLit]ast.Expr{}
	ast.Inspect(node, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for i, left := range value.Lhs {
				if i < len(value.Rhs) {
					recordProducerAssignment(left, value.Rhs[i], bound, assigned)
				}
			}
		case *ast.ValueSpec:
			for i, name := range value.Names {
				if i < len(value.Values) {
					recordProducerAssignment(name, value.Values[i], bound, assigned)
				}
			}
		case *ast.CompositeLit:
			if array, ok := value.Type.(*ast.ArrayType); ok {
				for _, element := range value.Elts {
					if literal, ok := element.(*ast.CompositeLit); ok && literal.Type == nil {
						inferred[literal] = array.Elt
					}
				}
			}
		}
		return true
	})
	var producers []textProducer
	ast.Inspect(node, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		kind := literal.Type
		if kind == nil {
			kind = inferred[literal]
		}
		if !producerContract(kind, local, packages, aliases, contracts) {
			return true
		}
		fields := assigned[bound[literal]]
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			field, ok := pair.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if field.Name == "SystemText" {
				fields.metadata = !isNilText(pair.Value)
			}
			fields.text = fields.text || isProducerTextField(field.Name)
		}
		producers = append(producers, textProducer{positions.Position(literal.Pos()).Line, !fields.text || fields.metadata})
		return true
	})
	return producers
}

func recordProducerAssignment(left, right ast.Expr, bound map[*ast.CompositeLit]string, assigned map[string]producerFields) {
	if name, ok := left.(*ast.Ident); ok {
		if literal := producerLiteral(right); literal != nil {
			bound[literal] = name.Name
		}
	}
	field, ok := left.(*ast.SelectorExpr)
	if !ok {
		return
	}
	name, ok := field.X.(*ast.Ident)
	if !ok {
		return
	}
	fields := assigned[name.Name]
	if field.Sel.Name == "SystemText" {
		fields.metadata = !isNilText(right)
	}
	fields.text = fields.text || isProducerTextField(field.Sel.Name)
	assigned[name.Name] = fields
}

func TestTheProducerCensusFindsHiddenHelperEmissions(t *testing.T) {
	t.Parallel()
	contracts, err := textProducerContracts(gatekit.ModuleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`func emit(){ sink(activities.LogActivityInput{Subject: new("Raw English task")}) }`,
		`var global = activities.LogActivityInput{Subject:new("English")}`,
		`func emit(){ sink(activities.LogActivityInput{Subject:new("English"), SystemText:&systemtext.Message{}}) }`,
		`type Alias = activities.LogActivityInput; func emit(){ sink(Alias{Subject:new("English")}) }`,
		`var batch = []activities.LogActivityInput{{Subject:new("English")}}`,
		`func emit(){ input := &activities.LogActivityInput{}; input.Subject = new("English"); sink(input) }`,
		`func emit(){ input := activities.LogActivityInput{}; input.SystemText=nil; input.Body=new("English"); sink(input) }`,
		`func emit(){ sink(activities.LogActivityInput{Subject:new("Raw English task"),SystemText:nil}) }`,
		`func emit(){ sink(activities.LogActivityInput{SystemText:metadata}); sink(activities.LogActivityInput{Subject:new("Raw English task")}) }`,
		`func emit(){ helper() }; func helper(){ sink(a.LogActivityInput{Body:new("Raw English instruction")}) }`,
	} {
		source := []byte(`package example; import activities "github.com/margince/margince/backend/internal/modules/activities"; import a "github.com/margince/margince/backend/internal/modules/activities"; ` + body)
		got, err := textProducers("fixture.go", source, contracts)
		if err != nil {
			t.Fatal(err)
		}
		unclassified := 0
		for _, producer := range got {
			if !producer.classified {
				unclassified++
			}
		}
		if unclassified != 1 {
			t.Fatalf("unclassified helper escaped: %+v", got)
		}
	}
}

func isNilText(expression ast.Expr) bool {
	if name, ok := expression.(*ast.Ident); ok {
		return name.Name == "nil"
	}
	literal := producerLiteral(expression)
	if literal == nil {
		return false
	}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		field, ok := pair.Key.(*ast.Ident)
		if ok && (field.Name == "Subject" || field.Name == "Body") && !isNilText(pair.Value) {
			return false
		}
	}
	return true
}
