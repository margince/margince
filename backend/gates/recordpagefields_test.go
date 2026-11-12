// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every sidecar field the company and contact pages render is one an agent can
// read through read_record and find in record_fields.
//
// The sidecar kinds are the fields of agents.RecordSidecars. Each is a crm.yaml
// schema whose `field` enum is its vocabulary. A page renders a kind when one of
// its sources types a value with that schema. It renders a key when a literal
// sits in a structure typed by the kind's key. That is a list or record whose
// type names the key, or the `field` prop of a component whose props declare it.
// A typed literal outside the vocabulary fails. So does a kind no page renders.
// A census with no VAT ID or no fact key fails too: the scan has stopped
// matching the pages.
//
// For each kind the pages render, compose/recordsidecarseam.go must assign its
// RecordSidecars field. read_record must declare the list it is answered in,
// and record_fields must list its whole vocabulary.

import (
	"context"
	"encoding/json"
	"go/ast"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const recordSidecarSeam = "internal/compose/recordsidecarseam.go"

// sidecarKind is one field of agents.RecordSidecars.
type sidecarKind struct {
	goField, schema, pageDir, answer string
	vocabulary                       []string
}

// recordPageDirs places a sidecar schema on the pages of the record it hangs off.
// gatekit:fixture the page folder of each record type
var recordPageDirs = map[string]string{
	"Company": "frontend/src/features/companies/",
	"Contact": "frontend/src/features/contacts/",
}

func readSidecarKinds(t *testing.T) []sidecarKind {
	t.Helper()
	enums := crmFieldEnums(t)
	shape := reflect.TypeFor[agents.RecordSidecars]()
	var kinds []sidecarKind
	for field := range shape.Fields() {
		if field.Type.Kind() != reflect.Slice {
			t.Fatalf("RecordSidecars.%s is not a list of rows; this gate reads each field as one", field.Name)
		}
		kind := sidecarKind{goField: field.Name, schema: field.Type.Elem().Name(), answer: "profile_fields"}
		if strings.HasSuffix(kind.schema, "Fact") {
			kind.answer = "facts"
		}
		for entity, dir := range recordPageDirs {
			if strings.HasPrefix(kind.schema, entity) {
				kind.pageDir = dir
			}
		}
		kind.vocabulary = enums[kind.schema]
		if kind.pageDir == "" || len(kind.vocabulary) == 0 {
			t.Fatalf("RecordSidecars.%s holds %s, which names no record page or has no `field` enum in crm.yaml",
				field.Name, kind.schema)
		}
		kinds = append(kinds, kind)
	}
	if len(kinds) == 0 {
		t.Fatal("agents.RecordSidecars has no fields")
	}
	return kinds
}

// crmFieldEnums reads each schema's `field` enum, and the ProfileFieldKey
// parameter, which the pages type the company profile keys with.
func crmFieldEnums(t *testing.T) map[string][]string {
	t.Helper()
	src, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading api/crm.yaml: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties struct {
					Field struct {
						Enum []string `yaml:"enum"`
					} `yaml:"field"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
			Parameters map[string]struct {
				Schema struct {
					Enum []string `yaml:"enum"`
				} `yaml:"schema"`
			} `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing api/crm.yaml: %v", err)
	}
	enums := map[string][]string{}
	for name, schema := range doc.Components.Schemas {
		if len(schema.Properties.Field.Enum) > 0 {
			enums[name] = schema.Properties.Field.Enum
		}
	}
	if !slices.Equal(doc.Components.Parameters["ProfileFieldKey"].Schema.Enum, enums["CompanyProfileField"]) {
		t.Fatal("crm.yaml's ProfileFieldKey and CompanyProfileField.field disagree; the pages type company " +
			"profile keys with the first and read_record answers the second")
	}
	return enums
}

var (
	pageSchemaUse     = regexp.MustCompile(`components\["schemas"\]\["(\w+)"\]`)
	pageRowAlias      = regexp.MustCompile(`type (\w+) = components\["schemas"\]\["(\w+)"\];`)
	pageKeyAlias      = regexp.MustCompile(`type (\w+) = (?:components\["schemas"\]\["(\w+)"\]|(\w+))\["field"\];`)
	pageParamKeyAlias = regexp.MustCompile(`type (\w+) = components\["parameters"\]\["ProfileFieldKey"\];`)
	pagePropsDecl     = regexp.MustCompile(`(?s)(?:type|interface) (\w+)\s*=?\s*(?:Readonly<)?\{(.*?)\n\}`)
	pageComponentDecl = regexp.MustCompile(`function (\w+)\(([^)]*)\)`)
	pageConstDecl     = regexp.MustCompile(`const (\w+)(?::([^=]+))?\s*=\s*([\[{])`)
	pageQuotedKey     = regexp.MustCompile(`"([a-z][a-z0-9_]*)"`)
)

// recordPageCensus is what the page sources render of each sidecar schema.
type recordPageCensus struct {
	rendered map[string]map[string]bool // page dir → schema
	keys     map[string]map[string]bool // schema → typed literal keys
	unplaced []string
}

func takeRecordPageCensus(kinds []sidecarKind, sources map[string]string) recordPageCensus {
	census := recordPageCensus{rendered: map[string]map[string]bool{}, keys: map[string]map[string]bool{}}
	vocab := map[string][]string{}
	for _, kind := range kinds {
		vocab[kind.schema] = kind.vocabulary
	}
	keyTypes, components := map[string]string{}, map[string]string{}
	for _, src := range sources {
		maps.Copy(keyTypes, pageKeyAliases(src, vocab))
	}
	for _, src := range sources {
		maps.Copy(components, pageKeyedComponents(src, keyTypes))
	}
	record := func(path, schema, key string) {
		if census.keys[schema] == nil {
			census.keys[schema] = map[string]bool{}
		}
		census.keys[schema][key] = true
		if !slices.Contains(vocab[schema], key) {
			census.unplaced = append(census.unplaced, path+": "+key+" is typed as a "+schema+" key")
		}
	}
	for path, src := range sources {
		for _, m := range pageSchemaUse.FindAllStringSubmatch(src, -1) {
			if _, sidecar := vocab[m[1]]; sidecar {
				dir := path[:strings.LastIndex(path, "/")+1]
				if census.rendered[dir] == nil {
					census.rendered[dir] = map[string]bool{}
				}
				census.rendered[dir][m[1]] = true
			}
		}
		for _, lit := range pageTypedLists(src, keyTypes) {
			record(path, lit.schema, lit.key)
		}
		for _, lit := range pageComponentFields(src, components) {
			record(path, lit.schema, lit.key)
		}
	}
	slices.Sort(census.unplaced)
	return census
}

// keyAliases names the type aliases a source spells a sidecar's key with.
func pageKeyAliases(src string, vocab map[string][]string) map[string]string {
	rows := map[string]string{}
	for _, m := range pageRowAlias.FindAllStringSubmatch(src, -1) {
		rows[m[1]] = m[2]
	}
	keys := map[string]string{}
	for _, m := range pageKeyAlias.FindAllStringSubmatch(src, -1) {
		schema := m[2]
		if schema == "" {
			schema = rows[m[3]]
		}
		if _, sidecar := vocab[schema]; sidecar {
			keys[m[1]] = schema
		}
	}
	for _, m := range pageParamKeyAlias.FindAllStringSubmatch(src, -1) {
		keys[m[1]] = "CompanyProfileField"
	}
	return keys
}

// keyedComponents names the components whose props declare `field` as a key type.
func pageKeyedComponents(src string, keyTypes map[string]string) map[string]string {
	props := map[string]string{}
	for _, m := range pagePropsDecl.FindAllStringSubmatch(src, -1) {
		for alias, schema := range keyTypes {
			if regexp.MustCompile(`\bfield\??:\s*` + alias + `\b`).MatchString(m[2]) {
				props[m[1]] = schema
			}
		}
	}
	out := map[string]string{}
	for _, m := range pageComponentDecl.FindAllStringSubmatch(src, -1) {
		for name, schema := range props {
			if regexp.MustCompile(`\b` + name + `\b`).MatchString(m[2]) {
				out[m[1]] = schema
			}
		}
	}
	return out
}

type pageTypedKey struct{ schema, key string }

// typedListLiterals reads the quoted literals of a const whose type annotation
// or `satisfies` clause names a key type.
func pageTypedLists(src string, keyTypes map[string]string) []pageTypedKey {
	var out []pageTypedKey
	for _, m := range pageConstDecl.FindAllStringSubmatchIndex(src, -1) {
		end := pageClosingBracket(src, m[6])
		if end < 0 {
			continue
		}
		typeText := ""
		if m[4] >= 0 {
			typeText = src[m[4]:m[5]]
		}
		if rest, ok := strings.CutPrefix(strings.TrimLeft(src[end+1:], " \n"), "as const"); ok {
			typeText += rest[:max(strings.IndexByte(rest, ';'), 0)]
		}
		for alias, schema := range keyTypes {
			if regexp.MustCompile(`\b` + alias + `\b`).MatchString(typeText) {
				for _, lit := range pageQuotedKey.FindAllStringSubmatch(src[m[6]:end], -1) {
					out = append(out, pageTypedKey{schema, lit[1]})
				}
			}
		}
	}
	return out
}

// componentFieldLiterals reads `field="…"` on the components keyed by a schema.
func pageComponentFields(src string, components map[string]string) []pageTypedKey {
	var out []pageTypedKey
	for name, schema := range components {
		for _, at := range regexp.MustCompile(`<`+name+`\b`).FindAllStringIndex(src, -1) {
			tag := src[at[1]:]
			if next := strings.IndexByte(tag, '<'); next >= 0 {
				tag = tag[:next]
			}
			if m := regexp.MustCompile(`\bfield="([a-z][a-z0-9_]*)"`).FindStringSubmatch(tag); m != nil {
				out = append(out, pageTypedKey{schema, m[1]})
			}
		}
	}
	return out
}

// pageClosingBracket answers the index of the bracket closing the one at open.
func pageClosingBracket(src string, open int) int {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// recordPageSources reads the production page sources, keyed by path.
func recordPageSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range trackedFiles(t) {
		for _, dir := range recordPageDirs {
			rel := strings.TrimPrefix(f.path, "frontend/src/")
			if strings.HasPrefix(f.path, dir) && !describesRatherThanRenders(rel) &&
				(strings.HasSuffix(f.path, ".tsx") || strings.HasSuffix(f.path, ".ts")) {
				out[f.path] = readSource(t, repoRoot+"/"+f.path)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no page source under the record page directories; the corpus has moved")
	}
	return out
}

// seamAssigns names the RecordSidecars fields the read_record seam assigns.
func seamAssigns(t *testing.T) map[string]bool {
	t.Helper()
	file, err := gatekit.ParseFile(recordSidecarSeam, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", recordSidecarSeam, err)
	}
	assigned := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					assigned[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
	return assigned
}

// recordFieldsNames reads the sidecar keys the record_fields document lists.
func recordFieldsNames(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := agents.RecordFieldsResource{}.RecordFieldsDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Parts map[string]json.RawMessage `json:"record_parts"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("reading record_fields: %v", err)
	}
	names := map[string]bool{}
	for _, section := range doc.Parts {
		var lists map[string]json.RawMessage
		if json.Unmarshal(section, &lists) != nil {
			continue
		}
		for _, list := range lists {
			var keys []string
			if json.Unmarshal(list, &keys) == nil {
				for _, key := range keys {
					names[key] = true
				}
			}
		}
	}
	return names
}

// readRecordAnswers names the lists read_record's output declares.
func readRecordAnswers(t *testing.T) map[string]bool {
	t.Helper()
	registry := agents.NewRegistry(nil, nil)
	agents.RegisterCoreTools(registry, nil, nil, nil, nil, nil, nil)
	spec, ok := registry.Spec("read_record")
	if !ok {
		t.Fatal("read_record is not registered")
	}
	var schema struct {
		Properties struct {
			Data struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"data"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.OutputSchema, &schema); err != nil {
		t.Fatalf("read_record's output schema: %v", err)
	}
	if len(schema.Properties.Data.Properties) == 0 {
		t.Fatal("read_record's output schema declares no data properties; the envelope this gate reads has moved")
	}
	answers := map[string]bool{}
	for name := range schema.Properties.Data.Properties {
		answers[name] = true
	}
	return answers
}

func TestEverySidecarFieldTheRecordPagesRenderIsOneAnAgentCanRead(t *testing.T) {
	t.Parallel()
	kinds := readSidecarKinds(t)
	census := takeRecordPageCensus(kinds, recordPageSources(t))
	for _, field := range census.unplaced {
		t.Errorf("%s, and crm.yaml's vocabulary for that schema does not hold it", field)
	}
	if !census.keys["CompanyProfileField"]["register_vat"] || len(census.keys["CompanyFact"]) == 0 {
		t.Fatalf("the census found company profile keys %v and fact keys %v; the pages render the VAT ID and "+
			"the stateable facts, so the scan has stopped matching them", census.keys["CompanyProfileField"], census.keys["CompanyFact"])
	}
	assigned, listed, answers := seamAssigns(t), recordFieldsNames(t), readRecordAnswers(t)
	for _, kind := range kinds {
		if !census.rendered[kind.pageDir][kind.schema] {
			t.Errorf("no source under %s renders a %s; the scan has stopped matching the page, or the kind "+
				"left it and RecordSidecars.%s should go too", kind.pageDir, kind.schema, kind.goField)
			continue
		}
		if !assigned[kind.goField] {
			t.Errorf("the %s page renders %s rows and %s never reads RecordSidecars.%s, so read_record leaves them out",
				kind.pageDir, kind.schema, recordSidecarSeam, kind.goField)
		}
		if !answers[kind.answer] {
			t.Errorf("the pages render %s rows and read_record declares no %s", kind.schema, kind.answer)
		}
		for _, key := range kind.vocabulary {
			if !listed[key] {
				t.Errorf("the pages render %s rows and record_fields does not list the key %q", kind.schema, key)
			}
		}
	}
}
