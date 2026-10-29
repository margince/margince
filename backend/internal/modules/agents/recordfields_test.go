// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The write tools' schemas are JSON handed to a client, and their field
// descriptions are built at init by splicing a constant into a hand-written
// literal — so the first thing to prove is that they are still parseable.
//
// The second is that the pointer still POINTS. The field vocabulary is
// published at RecordFieldsURI instead of being recited here, and the only
// thing standing between a caller and it is this sentence: a description that
// stopped naming the URI would leave the tool describing its `fields` argument
// as an opaque object with no way to learn a name — the trial-and-error surface
// the shapes were written to end, back with nothing reporting it. The cf_ shape
// stays in the description because it is what a caller needs BEFORE it decides
// to go and read.
func TestWriteToolSchemasPointAtThePublishedFieldVocabulary(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"create_record": createRecord{}.Spec().InputSchema,
		"update_record": updateRecord{}.Spec().InputSchema,
	} {
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("%s InputSchema is not valid JSON: %v\n%s", name, err, raw)
		}
		props, ok := parsed["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s InputSchema has no properties object", name)
		}
		fields, ok := props["fields"].(map[string]any)
		if !ok {
			t.Fatalf("%s InputSchema has no fields property", name)
		}
		desc, ok := fields["description"].(string)
		if !ok {
			t.Fatalf("%s fields property carries no description", name)
		}
		for _, want := range []string{RecordFieldsURI, "cf_<slug>"} {
			if !strings.Contains(desc, want) {
				t.Errorf("%s fields description does not mention %q: %s", name, want, desc)
			}
		}
	}
}

// The names come off the generated contract structs so they cannot drift from
// crm.yaml. That only holds while reflection actually reads json tags — a
// shape that answered no field names would render an empty, confidently wrong
// list, which is worse than the opaque object it replaced.
//
// Held by: TestContractFieldNamesReadsTheWireNames (backend/internal/modules/agents/recordfields_test.go) — this test.
func TestContractFieldNamesReadsTheWireNames(t *testing.T) {
	for recordType, shape := range createShapes {
		names := contractFieldNames(shape)
		if len(names) == 0 {
			t.Errorf("%s create shape yielded no field names", recordType)
		}
		for _, name := range names {
			if name == "-" || strings.Contains(name, ",") {
				t.Errorf("%s: %q is a raw struct tag, not a wire field name", recordType, name)
			}
		}
	}
	contact := strings.Join(contractFieldNames(createShapes["contact"]), ",")
	if !strings.Contains(contact, "full_name") || strings.Contains(contact, "display_name") {
		t.Errorf("contact create fields = %q, want the contract's full_name and no display_name", contact)
	}
}

// jsonString leans on Go and JSON string quoting agreeing, which they do for
// every character except a control character. Forbidding those here is what
// makes that lean safe instead of lucky — over the constants this package
// splices into a schema literal, whether they ride jsonString or are pasted in
// as pre-quoted JSON, since either spelling of a control character is a schema
// the client cannot parse.
func TestDescriptionsCarryNoControlCharacters(t *testing.T) {
	for name, desc := range map[string]string{
		"record fields": recordFieldsDescription,
		"timestamp":     timestampNote,
		"stage id":      stageIDNote,
	} {
		for i, r := range desc {
			if r < 0x20 || r == 0x7f {
				t.Errorf("%s description has control character %q at %d — Go would quote it in a form JSON rejects", name, r, i)
			}
		}
	}
}

// The writes real sessions lost data to. Each returned 200 with the value
// discarded: company_id is not a contact field at all, and `source` is a
// contact field on CREATE and no field at all on UPDATE — the shape a caller is
// most likely to get wrong, because it exists next door.
//
// Two cases have retired by being FIXED, which is the honest way for one here
// to stop being real: `emails` was added to the contact update so a bounced
// address could be corrected, and `phones` followed it so a reassigned number
// could be. `source` is the last create/update asymmetry the contact schema
// still carries — a patch's children are stamped `manual` because the request
// has no field to carry an origin (contacts/mapping.go) — so it is what this
// watches now. If the patch ever gains it, the case retires the same way, and
// whoever removes it should check whether any asymmetry is left to watch
// rather than letting the list quietly shrink.
func TestWriteToolsRefuseFieldsTheRecordCannotStore(t *testing.T) {
	cases := []struct {
		name       string
		shapes     writeShapes
		recordType string
		fields     string
		wantNamed  string
	}{
		{"company_id on a contact create", createWriteShapes, "contact", `{"full_name":"A","company_id":"x"}`, "company_id"},
		{"company_name on a contact create", createWriteShapes, "contact", `{"full_name":"A","company_name":"Acme"}`, "company_name"},
		{"source on a contact update", updateWriteShapes, "contact", `{"source":"manual"}`, "source"},
		{"company_id on a contact update", updateWriteShapes, "contact", `{"company_id":"x"}`, "company_id"},
		{"a typo next to a real field", createWriteShapes, "company", `{"displayname":"Firecrawl"}`, "displayname"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectUnknownFields(tc.shapes, tc.recordType, json.RawMessage(tc.fields))
			if err == nil {
				t.Fatalf("%s was accepted — the value would be discarded with no signal", tc.wantNamed)
			}
			if _, ok := errors.AsType[*BadArgsError](err); !ok {
				t.Fatalf("err = %v, want a BadArgsError the tool surface explains", err)
			}
			if !strings.Contains(err.Error(), tc.wantNamed) {
				t.Errorf("err = %q, want it to name %q", err, tc.wantNamed)
			}
			// Naming the field is half of it. The other half is that the
			// refusal still tells the caller what the type DOES accept —
			// otherwise it has replaced a silent drop with a dead end.
			if !strings.Contains(err.Error(), "accepts") {
				t.Errorf("err = %q, want it to list the fields the type accepts", err)
			}
			if !strings.Contains(err.Error(), datasource.CustomFieldPrefix) {
				t.Errorf("err = %q, want it to mention the custom-field channel", err)
			}
		})
	}
}

// What must still pass: real fields, and the custom-field channel. Whether a
// cf_ field is ACTIVE is the store's ratified question — refusing it here
// would break every workspace that defined one.
func TestWriteToolsAcceptRealFieldsAndTheCustomFieldChannel(t *testing.T) {
	for _, fields := range []string{
		`{"full_name":"Alex Nucci","title":"VP","emails":[{"email":"a@b.c"}]}`,
		`{"full_name":"Alex Nucci","cf_priority":"high"}`,
		`{}`,
	} {
		if err := rejectUnknownFields(createWriteShapes, "contact", json.RawMessage(fields)); err != nil {
			t.Errorf("rejectUnknownFields(%s) = %v, want accepted", fields, err)
		}
	}
	// An unknown record_type is the provider's refusal to make: it answers
	// with the vocabulary it serves, which this check cannot.
	if err := rejectUnknownFields(createWriteShapes, "invoice", json.RawMessage(`{"anything":1}`)); err != nil {
		t.Errorf("unknown record_type = %v, want it left to the provider", err)
	}
}

// Every schema argument declaring a date-time must splice in timestampNote or
// windowNote.
//
// Derived from the SOURCE of every schema(…) expression in this package, not
// from a list of tools: the first version of this gate hand-listed two tools
// and claimed "every", which is how a gate ends up certifying the four it never
// looked at. A new timestamp argument is covered the moment it is written.
func TestEveryTimestampArgumentDocumentsItsOffset(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "schema" {
				return true
			}
			literal := schemaText(call.Args[0])
			// PER OCCURRENCE, not per schema: an earlier version asked only
			// whether the schema referenced timestampNote anywhere, so a field
			// that lost its note passed on a sibling's. Every "date-time" must
			// be followed immediately by the note's splice.
			for _, occurrence := range dateTimeOccurrences(literal) {
				checked++
				if !occurrence {
					t.Errorf("%s:%d: a date-time argument in this schema does not splice timestampNote directly "+
						"after its format keyword — the keyword alone does not tell a caller an offset is required",
						name, fset.Position(call.Pos()).Line)
				}
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no schema in this package declares a date-time argument — the scan is broken, not the code")
	}
}

// The rendered input schema of every core tool, generated members included.
// The source census above reads only hand-written schema(…) literals. A
// struct-derived argument such as follow_up_task.due_at is invisible to it.
func TestEveryRenderedTimestampArgumentCarriesTheRule(t *testing.T) {
	rendered := map[string]string{}
	for name, tool := range idProbeDispatcher(t).registry.tools {
		var document any
		if err := json.Unmarshal(tool.Spec().InputSchema, &document); err != nil {
			t.Fatalf("%s: input schema is not JSON: %v", name, err)
		}
		collectDateTimes(document, name, rendered)
	}
	if len(rendered) == 0 {
		t.Fatal("no tool declares a date-time argument; the walk is broken, not the code")
	}
	for path, description := range rendered {
		if description != timestampRule && description != windowRule {
			t.Errorf("%s: a date-time argument without a timestamp rule: %q", path, description)
		}
	}
	// A recorded moment must not get an hour the user never said. A window or
	// a slot is chosen, so only its offset is fixed.
	for path, want := range map[string]string{
		"log_activity.properties.occurred_at":                      timestampRule,
		"log_activity.properties.due_at":                           timestampRule,
		"log_activity.properties.follow_up_task.properties.due_at": timestampRule,
		"check_availability.properties.from":                       windowRule,
		"check_availability.properties.to":                         windowRule,
		"book_meeting.properties.start":                            windowRule,
		"book_meeting.properties.end":                              windowRule,
	} {
		if rendered[path] != want {
			t.Errorf("%s carries %q, want %q", path, rendered[path], want)
		}
	}
}

// A model copies an example's offset into a day the user named without a
// time, so the example names UTC and no fixed offset.
func TestTheTimestampExampleCarriesNoFixedOffset(t *testing.T) {
	for name, rule := range map[string]string{"timestampRule": timestampRule, "windowRule": windowRule} {
		if offset := regexp.MustCompile(`[+-]\d\d:\d\d`).FindString(rule); offset != "" {
			t.Errorf("%s names the offset %s; a model copies it", name, offset)
		}
	}
	if !strings.Contains(timestampRule, "never invent an hour or offset") {
		t.Error("timestampRule must tell the model not to invent an hour")
	}
	if strings.Contains(windowRule, "hour") || !strings.Contains(windowRule, "never invent an offset") {
		t.Error("windowRule must fix only the offset; the model chooses a window's hour")
	}
}

// A day named without a time is the input that became a made-up noon. Each
// activity-writing tool tells the model what an omitted or moved time does.
func TestActivityWritersSayWhatAMissingTimeDoes(t *testing.T) {
	for name, rendered := range map[string]string{
		"log_activity":  logActivityCopy.render(),
		"create_record": createRecordCopy.render(),
	} {
		if !strings.Contains(rendered, pastDayNeedsATime) {
			t.Errorf("%s does not say how to send a past day with no time", name)
		}
	}
	if !strings.Contains(pastDayNeedsATime, `occurred_precision "day"`) {
		t.Error("a past day with no time must be sent as a day, not asked about or given a made-up hour")
	}
	if !strings.Contains(string(logActivity{}.Spec().InputSchema), `"occurred_precision"`) {
		t.Error("log_activity's schema does not offer the occurred_precision the copy tells the model to send")
	}
	update := updateRecordCopy.render()
	if !strings.Contains(update, "keeps its time of day unless the user names one") {
		t.Error("update_record does not say a moved activity keeps its time of day")
	}
	if !strings.Contains(update, `send occurred_precision "day" again`) {
		t.Error("update_record does not say a moved date-only activity must restate its precision")
	}
	if !strings.Contains(recordFieldsDescription, "carries Z or the offset the user gave") {
		t.Error("the fields description does not say where an rfc3339 offset comes from")
	}
}

// collectDateTimes records the description of every date-time member under
// its dotted path.
//
//craft:ignore naked-any a decoded JSON Schema document is any-shaped; a typed decode would drop the members this walk must see
func collectDateTimes(node any, path string, out map[string]string) {
	switch value := node.(type) {
	case map[string]any:
		if value["format"] == "date-time" {
			description, isText := value["description"].(string)
			if !isText {
				description = "(no description)"
			}
			out[path] = description
		}
		for key, member := range value {
			collectDateTimes(member, path+"."+key, out)
		}
	case []any:
		for _, member := range value {
			collectDateTimes(member, path, out)
		}
	}
}

// dateTimeOccurrences reports, for each `"format":"date-time"` in the flattened
// schema text, whether the note's splice follows it immediately. schemaText
// drops the interpolated expressions, so a spliced note leaves the literal
// halves adjoined as `…"date-time"}` — while a note that IS present leaves the
// marker text that timestampNote itself begins with.
func dateTimeOccurrences(literal string) []bool {
	const marker = `"format":"date-time"`
	var out []bool
	for i := 0; ; {
		at := strings.Index(literal[i:], marker)
		if at < 0 {
			return out
		}
		i += at + len(marker)
		out = append(out, strings.HasPrefix(literal[i:], `,"description":"RFC 3339`))
	}
}

// schemaText flattens a schema argument into the text a caller would receive.
// A schema is either one raw string or a `raw + const + raw` concatenation, so
// the spliced constants are substituted in rather than skipped — the gate reads
// the rendered result, not the source shape.
func schemaText(expr ast.Expr) string {
	var literal string
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch node := e.(type) {
		case *ast.BinaryExpr:
			walk(node.X)
			walk(node.Y)
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				literal += strings.Trim(node.Value, "`")
			}
		case *ast.Ident:
			// timestampNote is a string constant; splice its value in so the
			// per-occurrence check sees the text a caller would receive.
			switch node.Name {
			case "timestampNote":
				literal += timestampNote
			case "windowNote":
				literal += windowNote
			}
		case *ast.CallExpr:
			for _, arg := range node.Args {
				walk(arg)
			}
			walk(node.Fun)
		}
	}
	walk(expr)
	return literal
}

// The prefix is not a slug. `cf_` alone names no catalog column, so accepting
// it would let the one key the check is meant to catch through the one hole it
// leaves — and a null payload decodes to a nil map with no error and no keys at
// all, which is the same hole wearing a different shape.
func TestWriteToolsRefuseTheCustomFieldPrefixWithNoSlug(t *testing.T) {
	for name, fields := range map[string]string{
		"bare prefix": `{"full_name":"A","cf_":"x"}`,
		"null body":   `null`,
	} {
		t.Run(name, func(t *testing.T) {
			err := rejectUnknownFields(createWriteShapes, "contact", json.RawMessage(fields))
			if err == nil {
				t.Fatalf("%s was accepted — the value would be discarded with no signal", fields)
			}
			if _, ok := errors.AsType[*BadArgsError](err); !ok {
				t.Errorf("err = %v, want a BadArgsError the tool surface explains", err)
			}
		})
	}
	// A real slug still passes: whether that field is ACTIVE is the store's
	// question, and refusing it here would break every workspace that has one.
	if err := rejectUnknownFields(createWriteShapes, "contact", json.RawMessage(`{"full_name":"A","cf_priority":"high"}`)); err != nil {
		t.Errorf("a real custom-field key was refused: %v", err)
	}
}

// The record_type enum a write tool advertises and the shapes it can actually
// decode are two spellings of one set, written in two places — a literal
// inside the schema string, and the keys of the shapes map. §8.2b of
// NEXT-MCP-MISSING is what happens when they drift: the contract declared
// update_record/activity, the shapes map deliberately omitted it to match an
// enum that omitted it too, and an activity became un-updatable over the tool
// surface while REST served the patch fine.
//
// Derived rather than listed, so a record type added to either place fails
// here until it is added to both.
func TestWriteToolEnumsMatchTheShapesTheyCanDecode(t *testing.T) {
	for _, tc := range []struct {
		tool   string
		schema json.RawMessage
		shapes map[datasource.EntityType]reflect.Type
	}{
		{"create_record", createRecord{}.Spec().InputSchema, createShapes},
		{"update_record", updateRecord{}.Spec().InputSchema, updateShapes},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			advertised := recordTypeEnum(t, tc.tool, tc.schema)
			decodable := make(map[string]bool, len(tc.shapes))
			for recordType := range tc.shapes {
				decodable[string(recordType)] = true
			}

			for _, recordType := range advertised {
				if !decodable[recordType] {
					t.Errorf("%s advertises record_type %q but describes no fields for it — "+
						"a caller reading the enum would send a patch the description never explains",
						tc.tool, recordType)
				}
				if !slices.Contains(datasource.EntityTypes(), datasource.EntityType(recordType)) {
					t.Errorf("%s advertises record_type %q, which is outside the datasource vocabulary — "+
						"no provider could serve it", tc.tool, recordType)
				}
			}
			for recordType := range decodable {
				if !slices.Contains(advertised, recordType) {
					t.Errorf("%s can decode %q but does not advertise it — the capability exists and no caller can reach it",
						tc.tool, recordType)
				}
			}
		})
	}
}

// recordTypeEnum reads the record_type enum out of a tool's advertised schema,
// which is the string a client actually reads.
func recordTypeEnum(t *testing.T, tool string, raw json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Properties struct {
			RecordType struct {
				Enum []string `json:"enum"`
			} `json:"record_type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("%s InputSchema is not valid JSON: %v", tool, err)
	}
	if len(parsed.Properties.RecordType.Enum) == 0 {
		t.Fatalf("%s advertises no record_type enum — this walk would pass vacuously", tool)
	}
	return parsed.Properties.RecordType.Enum
}

// A refusal hands back the SHAPE, because the name alone leaves the next
// refusal in place.
//
// Measured: a run told `company` accepts `domains` sent
// `["example.test"]`, was refused for an array of strings, and only then
// learned the item shape; another omitted `relationship.kind` because nothing
// had said which fields are required. Both facts are in the generated line the
// tool description deliberately does not recite — reciting it THERE costs every
// call, and a refusal costs only the caller who needed it.
func TestARefusedFieldListCarriesTypesAndWhatIsRequired(t *testing.T) {
	t.Parallel()
	cases := []struct {
		what   string
		shapes writeShapes
		record string
		fields string
		says   []string
	}{
		{
			what: "a company create", shapes: createWriteShapes, record: "company",
			fields: `{"name":"Terralogic","domain":"terralogic.test"}`,
			says: []string{
				// The item shape, which a name list cannot carry.
				"domains?: [{domain: string, is_primary?: boolean}]",
				// Required, marked by the ABSENCE of a `?`.
				"display_name: string",
			},
		},
		{
			what: "a relationship create", shapes: createWriteShapes, record: "relationship",
			fields: `{"who":"x"}`,
			// The closed vocabulary of a required field, in the same line.
			says: []string{`kind: "employment"|`, "contact_id?: uuid"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			err := rejectUnknownFields(tc.shapes, tc.record, json.RawMessage(tc.fields))
			var bad *BadArgsError
			if !errors.As(err, &bad) {
				t.Fatalf("err = %v, want a BadArgsError", err)
			}
			for _, want := range tc.says {
				if !strings.Contains(bad.Guidance, want) {
					t.Errorf("the guidance does not carry %q, so the caller learns the name and "+
						"is refused again on the shape:\n%s", want, bad.Guidance)
				}
			}
		})
	}
}

// And an update's shape is the UPDATE's, not the create's — the two differ, and
// handing back the wrong one sends a caller to a field this door does not take.
func TestARefusedUpdateCarriesTheUpdateShape(t *testing.T) {
	t.Parallel()
	err := rejectUnknownFields(updateWriteShapes, "relationship", json.RawMessage(`{"who":"x"}`))
	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("err = %v, want a BadArgsError", err)
	}
	if strings.Contains(bad.Guidance, "kind:") {
		t.Errorf("an update-door refusal offers `kind`, which only the create body takes:\n%s", bad.Guidance)
	}
	if !strings.Contains(bad.Guidance, "role?: string") {
		t.Errorf("the update-door refusal does not carry that door's own shape:\n%s", bad.Guidance)
	}
}
