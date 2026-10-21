// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Home repair labels cover the reasons emitted by its canonical classifiers and reviewed projections.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type repairReasonCensus struct {
	constants map[string]ast.Expr
	functions map[string]*ast.FuncDecl
	reasons   map[string]bool
	unread    []string
}

func TestHomeRepairExplainsEveryOwnerReason(t *testing.T) {
	t.Parallel()
	census := repairReasonCensus{constants: map[string]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, reasons: map[string]bool{}}
	var files []*ast.File
	for _, directory := range []string{"internal/compose", "internal/compose/lifecyclereview", "internal/modules/activities", "internal/modules/deals", "internal/platform/auth"} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := gatekit.ParseFile(filepath.Join(directory, entry.Name()), 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, file)
			census.index(file)
		}
	}
	for _, file := range files {
		census.read(file)
	}
	if len(census.unread) != 0 {
		t.Fatalf("repair reason census cannot resolve: %v", census.unread)
	}
	if len(census.reasons) == 0 {
		t.Fatal("repair reason census is empty")
	}
	source, err := os.ReadFile("../frontend/src/features/settings/homerepair.tsx")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "export const HOME_REPAIR_REASON_LABELS")
	if start < 0 {
		t.Fatal("repair label map is missing")
	}
	body := string(source)[start:]
	body, _, found := strings.Cut(body, "\n};")
	if !found {
		t.Fatal("repair label map is incomplete")
	}
	labels := map[string]bool{}
	for _, match := range tsReasonEntry.FindAllStringSubmatch(tsReasonComment.ReplaceAllString(body, ""), -1) {
		labels[match[1]] = true
	}
	for label := range labels {
		if !census.reasons[label] {
			t.Errorf("frontend label %q has no repair reason owner", label)
		}
	}
	for reason := range census.reasons {
		if !labels[reason] {
			t.Errorf("repair reason %q has no frontend label", reason)
		}
	}
}

func (c *repairReasonCensus) index(file *ast.File) {
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			c.functions[file.Name.Name+"."+fn.Name.Name] = fn
		}
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		var inherited []ast.Expr
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			expressions := value.Values
			if len(expressions) == 0 {
				expressions = inherited
			} else {
				inherited = expressions
			}
			for i, name := range value.Names {
				if i < len(expressions) {
					c.constants[file.Name.Name+"."+name.Name] = expressions[i]
				}
			}
		}
	}
}

func (c *repairReasonCensus) read(file *ast.File) {
	for _, declaration := range file.Decls {
		if group, ok := declaration.(*ast.GenDecl); ok && group.Tok == token.CONST {
			c.readVocabulary(file.Name.Name, group)
		}
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || (file.Name.Name != "compose" && file.Name.Name != "deals") || !repairFunction(fn) {
			continue
		}
		// Skip-audit reasons never enter the review response; the card only reads Reason fields.
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch item := node.(type) {
			case *ast.KeyValueExpr:
				if name, ok := item.Key.(*ast.Ident); ok && name.Name == "Reason" {
					c.reason(item.Value, file.Name.Name+"."+fn.Name.Name, 0)
				}
			case *ast.AssignStmt:
				for i, lhs := range item.Lhs {
					field, ok := lhs.(*ast.SelectorExpr)
					if !ok || field.Sel.Name != "Reason" {
						continue
					}
					switch {
					case len(item.Rhs) == len(item.Lhs):
						c.reason(item.Rhs[i], file.Name.Name+"."+fn.Name.Name, 0)
					case len(item.Rhs) == 1:
						c.reasonResult(item.Rhs[0], i, file.Name.Name+"."+fn.Name.Name, 0)
					default:
						c.unread = append(c.unread, fn.Name.Name+": unmatched reason assignment")
					}
				}
			case *ast.ReturnStmt:
				if repairActionResult(fn) && len(item.Results) > 1 {
					c.reason(item.Results[1], file.Name.Name+"."+fn.Name.Name, 0)
				}
			}
			return true
		})
	}
}

// These projections forward values from the separately censused classifier or reviewed scope.
func repairReasonForward(owner, receiver string) bool {
	switch owner {
	case "compose.homeRepairSuggestionRef":
		return receiver == "repair"
	case "compose.applyHomeRepairSuggestion":
		return receiver == "expected"
	case "compose.previewHomeRepairSourceTx":
		return receiver == "decision"
	case "compose.homeRepairReviewWire", "compose.homeRepairReviewInput":
		return receiver == "task"
	default:
		return false
	}
}

func repairFunction(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(node ast.Node) bool {
		if name, ok := node.(*ast.Ident); ok && (strings.HasPrefix(name.Name, "homeRepair") || name.Name == "SuggestionRepair") {
			found = true
		}
		return true
	})
	return found
}

func repairActionResult(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}
	name, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && name.Name == "homeRepairAction"
}

func (c *repairReasonCensus) readVocabulary(pkg string, group *ast.GenDecl) {
	admission := false
	for _, spec := range group.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if name, ok := value.Type.(*ast.Ident); ok && name.Name == "CommitmentAdmissionReason" {
			admission = true
		}
	}
	for _, spec := range group.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			if admission || strings.HasPrefix(name.Name, "RepairReason") {
				c.resolve(pkg+"."+name.Name, "vocabulary", 0)
			}
		}
	}
}

func (c *repairReasonCensus) resolve(name, owner string, depth int) {
	expression, ok := c.constants[name]
	if !ok || depth > 20 {
		c.unread = append(c.unread, owner+": "+name)
		return
	}
	if literal, ok := expression.(*ast.BasicLit); ok && literal.Value == `""` && name != "activities.CommitmentCurrent" {
		c.unread = append(c.unread, owner+": unexpected empty reason "+name)
		return
	}
	c.reason(expression, owner, depth+1)
}

func (c *repairReasonCensus) reason(expression ast.Expr, owner string, depth int) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		text, err := strconv.Unquote(value.Value)
		if err != nil {
			c.unread = append(c.unread, owner+": invalid literal")
			return
		}
		if text != "" {
			c.reasons[text] = true
		}
	case *ast.Ident:
		// These variables forward the admission owner whose complete vocabulary is read above.
		if value.Name == "reason" && (owner == "compose.homeRepairCommitmentAdmissionTx" || owner == "compose.homeRepairCommitmentProposalTx") {
			return
		}
		c.resolve(strings.Split(owner, ".")[0]+"."+value.Name, owner, depth)
	case *ast.SelectorExpr:
		qualifier, ok := value.X.(*ast.Ident)
		if !ok {
			c.unread = append(c.unread, owner+": nested reason selector")
			return
		}
		if value.Sel.Name == "Reason" && repairReasonForward(owner, qualifier.Name) {
			return
		}
		c.resolve(qualifier.Name+"."+value.Sel.Name, owner, depth)
	case *ast.CallExpr:
		name, ok := value.Fun.(*ast.Ident)
		if ok && name.Name == "string" && len(value.Args) == 1 {
			c.reason(value.Args[0], owner, depth)
			return
		}
		c.reasonResult(value, 0, owner, depth)
	default:
		c.unread = append(c.unread, owner+": unresolved reason expression")
	}
}

func (c *repairReasonCensus) reasonResult(expression ast.Expr, index int, owner string, depth int) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || depth > 20 {
		c.unread = append(c.unread, owner+": unresolved reason result")
		return
	}
	name, ok := call.Fun.(*ast.Ident)
	if !ok || c.functions[strings.Split(owner, ".")[0]+"."+name.Name] == nil {
		c.unread = append(c.unread, owner+": unresolved reason function")
		return
	}
	qualified := strings.Split(owner, ".")[0] + "." + name.Name
	fn := c.functions[qualified]
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if result, ok := node.(*ast.ReturnStmt); ok && len(result.Results) > index {
			found = true
			c.reason(result.Results[index], qualified, depth+1)
		}
		return true
	})
	if !found {
		c.unread = append(c.unread, owner+": reason function has no readable return")
	}
}

func TestHomeRepairReasonCensusReadsConstantsTuplesAndHelpers(t *testing.T) {
	t.Parallel()
	source := `package compose
 const reasonHidden = "constant_reason"
 func nextReason() string { return "helper_reason" }
 func decide() homeRepairTaskRef {
  item := homeRepairTaskRef{Reason: reasonHidden}
  item.Reason, item.Action = "tuple_reason", action
  item.Reason = nextReason()
  return item
 }
 func admission() (homeRepairAction, string, error) { return action, "return_reason", nil }
 `
	file, err := parser.ParseFile(token.NewFileSet(), "renamed.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	census := repairReasonCensus{constants: map[string]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, reasons: map[string]bool{}}
	census.index(file)
	census.read(file)
	if len(census.unread) != 0 {
		t.Fatal(census.unread)
	}
	for _, reason := range []string{"constant_reason", "tuple_reason", "helper_reason", "return_reason"} {
		if !census.reasons[reason] {
			t.Errorf("missed %s", reason)
		}
	}
}

func TestHomeRepairReasonCensusRefusesUnreadableValues(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`item := homeRepairTaskRef{Reason: unknownReason}; return item`,
		`item := homeRepairTaskRef{}; item.Reason = unknownReason(); return item`,
		`item := homeRepairTaskRef{}; item.Reason = other.Reason; return item`,
		`item := homeRepairTaskRef{}; item.Reason = decision.Reason; return item`,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "othername.go", "package compose\nfunc f() homeRepairTaskRef {"+body+"}", 0)
		if err != nil {
			t.Fatal(err)
		}
		census := repairReasonCensus{constants: map[string]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, reasons: map[string]bool{}}
		census.index(file)
		census.read(file)
		if len(census.unread) == 0 {
			t.Errorf("silently omitted %s", body)
		}
	}
}

func TestHomeRepairReasonCensusReadsUntypedVocabularyMembers(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "renamed.go", `package activities
 const (
 CommitmentCurrent CommitmentAdmissionReason = ""
 NewReason = "new_reason"
 InheritedReason
 )`, 0)
	if err != nil {
		t.Fatal(err)
	}
	census := repairReasonCensus{constants: map[string]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, reasons: map[string]bool{}}
	census.index(file)
	census.read(file)
	if len(census.unread) != 0 || !census.reasons["new_reason"] {
		t.Fatalf("vocabulary census: reasons=%v unread=%v", census.reasons, census.unread)
	}
}

func TestHomeRepairReasonCensusReadsSuggestionWriterValues(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "renamed.go", `package deals
 const stale = "unnamed_constant_reason"
 func decide() SuggestionRepair {
  out := SuggestionRepair{Reason: stale}
  out.Reason = "literal_reason"
  return out
 }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	census := repairReasonCensus{constants: map[string]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, reasons: map[string]bool{}}
	census.index(file)
	census.read(file)
	if len(census.unread) != 0 {
		t.Fatal(census.unread)
	}
	for _, reason := range []string{"unnamed_constant_reason", "literal_reason"} {
		if !census.reasons[reason] {
			t.Errorf("missed suggestion writer reason %s", reason)
		}
	}
}
