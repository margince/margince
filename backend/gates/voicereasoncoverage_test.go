// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var voiceReasonBranch = regexp.MustCompile(`(?:case\s+["']([a-z_]+)["']\s*:|code\s*===\s*["']([a-z_]+)["'])`)

func TestEveryVoiceReviewCodeHasAClientExplanation(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "internal/modules/ai/voice_review_codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	names := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			t.Fatal("voice reason catalog must declare explicit codes")
		}
		literal, ok := declaration.Values[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			t.Fatal("voice reason catalog contains a nonliteral code")
		}
		code, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		codes = append(codes, code)
		names[declaration.Names[0].Name] = true
		return true
	})
	if len(codes) == 0 {
		t.Fatal("voice reason catalog is empty")
	}
	assertVoiceReasonWriters(t, names)
	source, err := os.ReadFile("../frontend/src/features/voice-knowledge/voice-review-reason.ts")
	if err != nil {
		t.Fatal(err)
	}
	text := tsReasonComment.ReplaceAllString(string(source), " ")
	start := strings.Index(text, "function plainVoiceReason(")
	end := strings.Index(text, "function legacyReviewReason(")
	if start < 0 || end <= start {
		t.Fatal("typed voice renderer boundaries are missing")
	}
	rendered := voiceReasonBranches(text[start:end])
	slices.Sort(codes)
	if !slices.Equal(codes, rendered) {
		t.Fatalf("voice reason codes %v differ from client explanations %v", codes, rendered)
	}
}

func voiceReasonBranches(source string) []string {
	var codes []string
	for _, match := range voiceReasonBranch.FindAllStringSubmatch(tsReasonComment.ReplaceAllString(source, " "), -1) {
		codes = append(codes, match[1]+match[2])
	}
	slices.Sort(codes)
	return slices.Compact(codes)
}

func TestVoiceReasonCensusReadsBranchesRatherThanComments(t *testing.T) {
	t.Parallel()
	source := `// case "comment_only":
 if (code === "scored" && finite(values.score)) { return t("label"); }
 switch (code) { case 'plain': return t("label"); }
 /* if (code === "removed") {} */`
	if got := voiceReasonBranches(source); !slices.Equal(got, []string{"plain", "scored"}) {
		t.Fatalf("reason branches = %v", got)
	}
}

func assertVoiceReasonWriters(t *testing.T, catalog map[string]bool) {
	t.Helper()
	scope := gatekit.Scope{Roots: []string{"internal"}, Exempt: gatekit.Waive(map[string]string{}), Subject: func(_ string, file *ast.File) bool {
		return len(voiceReasonCodes(file)) > 0
	}}
	for _, file := range scope.Files(t) {
		for _, code := range voiceReasonCodes(file.File) {
			name := ""
			switch value := code.(type) {
			case *ast.Ident:
				name = value.Name
			case *ast.SelectorExpr:
				name = value.Sel.Name
			}
			if !catalog[name] {
				t.Errorf("%s: voice reason writer must use a catalog code, got %T %s", file.Path, code, name)
			}
		}
	}
}

func voiceReasonCodes(file *ast.File) []ast.Expr {
	var codes []ast.Expr
	for _, declaration := range file.Decls {
		function, isFunction := declaration.(*ast.FuncDecl)
		var parents []ast.Node
		ast.Inspect(declaration, func(node ast.Node) bool {
			if node == nil {
				parents = parents[:len(parents)-1]
				return true
			}
			var parent ast.Node
			if len(parents) > 0 {
				parent = parents[len(parents)-1]
			}
			parents = append(parents, node)
			if call, ok := node.(*ast.CallExpr); ok {
				if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "appendReviewReason" && len(call.Args) > 0 {
					codes = append(codes, call.Args[0])
				}
			}
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			kind := literal.Type
			if kind == nil {
				if container, ok := parent.(*ast.CompositeLit); ok {
					if array, ok := container.Type.(*ast.ArrayType); ok {
						kind = array.Elt
					}
				}
			}
			name := ""
			switch kind := kind.(type) {
			case *ast.SelectorExpr:
				name = kind.Sel.Name
			case *ast.Ident:
				name = kind.Name
			}
			if name != "VoiceReviewReason" {
				return true
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := field.Key.(*ast.Ident)
				if !ok || key.Name != "Code" {
					continue
				}
				// The forwarding method's argument is checked at every call above.
				if parameter, ok := field.Value.(*ast.Ident); ok && parameter.Name == "code" && isFunction && function.Name.Name == "appendReviewReason" {
					continue
				}
				codes = append(codes, field.Value)
			}
			return true
		})
	}
	return codes
}

func TestVoiceReasonCensusFindsLiteralAndForwardedCodes(t *testing.T) {
	t.Parallel()
	source := `package example
 var direct = VoiceReviewReason{Code: "global_literal"}
 var batch = []crm.VoiceReviewReason{{Code: "inferred_literal"}}
 func write() {
  _ = crm.VoiceReviewReason{Code: "outside_catalog"}
  outcome.appendReviewReason(ai.VoiceReasonCorpusChanged, "explanation")
 }
 func (outcome *Outcome) appendReviewReason(code, text string) {
  _ = crm.VoiceReviewReason{Code: code, Text: text}
 }`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	codes := voiceReasonCodes(file)
	if len(codes) != 4 {
		t.Fatalf("writer census found %d codes, want global, inferred, direct and forwarded writers", len(codes))
	}
	literal, ok := codes[2].(*ast.BasicLit)
	if !ok {
		t.Fatal("direct code is not a literal")
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil || text != "outside_catalog" {
		t.Fatalf("literal writer not found: %v", codes[2])
	}
	selector, ok := codes[3].(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "VoiceReasonCorpusChanged" {
		t.Fatalf("forwarded writer not found: %v", codes[3])
	}
}
