// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A gate decodes YAML through unmarshalYAML, which parses each large document
// once for the whole run.
//
// The contract alone is 4.5 MB, and about seventy decodes read it. A gate that
// calls yaml.Unmarshal itself still passes, so the cost shows only as a slower
// run. The census sees a reference under any import name.

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const yamlImportPath = "gopkg.in/yaml.v3"

// sharedYAMLMinSize keeps small documents out of the held set: below it, a
// parse costs less than holding its tree for the rest of the run.
const sharedYAMLMinSize = 32 << 10

type yamlParse struct {
	once sync.Once
	doc  yaml.Node
	err  error
}

// yamlParses is keyed by the document text. Two readers of one file share a
// tree however they spell its path, and an edited file never meets a stale one.
var yamlParses struct {
	sync.Mutex
	byText map[string]*yamlParse
}

// unmarshalYAML answers what yaml.Unmarshal answers. A large document is parsed
// once and each call decodes the held tree into its own value.
//
//craft:ignore naked-any it takes what yaml.Unmarshal takes, so every call site keeps its own typed value
func unmarshalYAML(raw []byte, out any) error {
	if len(raw) < sharedYAMLMinSize || !decodesWithoutNodes(reflect.TypeOf(out)) {
		return yaml.Unmarshal(raw, out)
	}
	held := heldYAML(raw)
	held.once.Do(func() { held.err = yaml.Unmarshal(raw, &held.doc) })
	if held.err != nil {
		return held.err
	}
	if held.doc.IsZero() {
		return nil // no document: yaml.Unmarshal leaves out as it was
	}
	return held.doc.Decode(out)
}

func heldYAML(raw []byte) *yamlParse {
	yamlParses.Lock()
	defer yamlParses.Unlock()
	if held, ok := yamlParses.byText[string(raw)]; ok {
		return held
	}
	if yamlParses.byText == nil {
		yamlParses.byText = map[string]*yamlParse{}
	}
	held := &yamlParse{}
	yamlParses.byText[string(raw)] = held
	return held
}

var (
	yamlNodeType    = reflect.TypeFor[yaml.Node]()
	yamlUnmarshaler = reflect.TypeFor[yaml.Unmarshaler]()
)

// decodesWithoutNodes reports whether decoding into typ copies values out of
// the tree. A yaml.Node field or an UnmarshalYAML method gets a held node, which
// its caller could change under every later reader.
func decodesWithoutNodes(typ reflect.Type) bool {
	return typ != nil && holdsNoNode(typ, map[reflect.Type]bool{})
}

func holdsNoNode(typ reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[typ] {
		return true
	}
	seen[typ] = true
	if typ == yamlNodeType || typ.Implements(yamlUnmarshaler) || reflect.PointerTo(typ).Implements(yamlUnmarshaler) {
		return false
	}
	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return holdsNoNode(typ.Elem(), seen)
	case reflect.Map:
		return holdsNoNode(typ.Key(), seen) && holdsNoNode(typ.Elem(), seen)
	case reflect.Struct:
		for i := range typ.NumField() {
			if !holdsNoNode(typ.Field(i).Type, seen) {
				return false
			}
		}
	}
	return true
}

// directYAMLDecodes is keyed "path:function".
var directYAMLDecodes = gatekit.Waive(map[string]string{
	"gates/sharedyaml_test.go:unmarshalYAML": "is the shared parse itself, and parses a small document or a node-holding value directly",
	"gates/openchannelinboundschema_test.go:readOpenAPIDoc": "is in the external test package, which cannot call " +
		"unmarshalYAML, and decodes one small extension contract",
	"gates/sharedyaml_test.go:TestASharedDecodeAnswersWhatADirectDecodeAnswers": "compares the shared parse " +
		"against yaml.Unmarshal itself",
})

// Below the real count of about ninety decodes.
const sharedYAMLDecodeFloor = 80

func TestEveryGateDecodesYAMLThroughTheSharedParse(t *testing.T) {
	t.Parallel()
	shared := 0
	for _, path := range sharedTreeCensusFiles(t) {
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		yamlName, imported := yamlImportName(file)
		for _, decl := range file.Decls {
			ast.Inspect(decl, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.Ident:
					if node.Name == "unmarshalYAML" {
						shared++
					}
				case *ast.SelectorExpr:
					pkg, ok := node.X.(*ast.Ident)
					if !imported || !ok || pkg.Name != yamlName || node.Sel.Name != "Unmarshal" {
						return true
					}
					if directYAMLDecodes.Waived(t, filepath.ToSlash(path)+":"+declName(decl)) {
						return true
					}
					t.Errorf("%s:%d: %s calls yaml.Unmarshal: use unmarshalYAML, which answers the same and parses "+
						"a large document once for every gate in the run", filepath.ToSlash(path),
						gatekit.SourceFileSet().Position(node.Pos()).Line, declName(decl))
				}
				return true
			})
		}
	}
	if shared < sharedYAMLDecodeFloor {
		t.Errorf("the census found %d references to unmarshalYAML, below the floor of %d: the matcher stopped "+
			"recognising them, so it no longer proves it can see a decode at all", shared, sharedYAMLDecodeFloor)
	}
	directYAMLDecodes.AssertAllMatched(t)
}

// yamlImportName is the name a file calls the YAML package by.
func yamlImportName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != yamlImportPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true
		}
		return "yaml", true
	}
	return "", false
}

func TestASharedDecodeAnswersWhatADirectDecodeAnswers(t *testing.T) {
	t.Parallel()
	raw := []byte(readFile(t, filepath.Join("api", "crm.yaml")))
	if len(raw) < sharedYAMLMinSize {
		t.Fatalf("api/crm.yaml is %d bytes, below the %d the shared parse holds: this test no longer reaches it", len(raw), sharedYAMLMinSize)
	}
	type contract struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	var direct, first, second contract
	if err := yaml.Unmarshal(raw, &direct); err != nil {
		t.Fatalf("decoding api/crm.yaml: %v", err)
	}
	for _, out := range []*contract{&first, &second} {
		if err := unmarshalYAML(raw, out); err != nil {
			t.Fatalf("decoding api/crm.yaml through the shared parse: %v", err)
		}
	}
	if !reflect.DeepEqual(direct, first) || !reflect.DeepEqual(direct, second) {
		t.Fatal("a decode from the held tree differs from yaml.Unmarshal of the same text")
	}
	delete(first.Paths, firstKey(first.Paths))
	if reflect.DeepEqual(first, second) {
		t.Fatal("two decodes from the held tree share their maps, so one reader's change reaches the next")
	}
}

func TestANodeHoldingValueIsNeverDecodedFromTheHeldTree(t *testing.T) {
	t.Parallel()
	type withNode struct {
		Info yaml.Node `yaml:"info"`
	}
	for _, out := range []any{&yaml.Node{}, &withNode{}, &map[string]yaml.Node{}, &[]*yaml.Node{}} {
		if decodesWithoutNodes(reflect.TypeOf(out)) {
			t.Errorf("%T holds a yaml.Node, so a shared decode would hand every caller the same node", out)
		}
	}
	if !decodesWithoutNodes(reflect.TypeOf(&map[string]any{})) {
		t.Error("a map of plain values holds no node, and should decode from the held tree")
	}
}

func firstKey[V any](m map[string]V) string {
	for key := range m {
		return key
	}
	return ""
}
