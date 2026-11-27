// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// Each text cap the server applies is a hand-typed mirror of a number
// `api/crm.yaml` publishes, because the generated server enforces no string
// length. This gate fails the day one of them drifts. Raise the contract alone
// and the server refuses what the schema calls valid. Raise the constant alone
// and a client truncates to a bound nobody applies. A cap joins the list when a
// package copies the number instead of reading it.

import (
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type textLimit struct {
	file     string
	constant string
	// path walks the parsed contract to the schema node carrying maxLength.
	path []string
}

func schemaPath(parts ...string) []string { return parts }

var textLimits = []textLimit{
	{
		"internal/compose/bulkvalidate.go", "maxBulkTaskSubject",
		schemaPath("components", "schemas", "BulkTask", "properties", "subject"),
	},
	{
		"internal/compose/bulkvalidate.go", "maxBulkNote",
		schemaPath("components", "schemas", "BulkChangeExecuteRequest", "properties", "note"),
	},
	{
		"internal/compose/bulkvalidate.go", "maxBulkNote",
		schemaPath("components", "schemas", "BulkChangePreviewRequest", "properties", "note"),
	},
	{
		"internal/modules/activities/maildraft.go", "maxDraftSubject",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "subject"),
	},
	{
		"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "to", "items"),
	},
	{
		"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "cc", "items"),
	},
	{
		"internal/modules/activities/maildraft.go", "maxDraftAddress",
		schemaPath("components", "schemas", "MailDraftInput", "properties", "bcc", "items"),
	},
	{
		"internal/modules/collections/memberwrite.go", "maxMemberNote",
		schemaPath("components", "schemas", "ListMemberChangeRequest", "properties", "note"),
	},
	{
		"internal/modules/introductions/store.go", "noteBound",
		schemaPath("components", "schemas", "IntroRequestInput", "properties", "forwardable_note"),
	},
	{
		"internal/modules/introductions/store.go", "reasonBound",
		schemaPath("components", "schemas", "IntroRequestInput", "properties", "value_for_target"),
	},
	{"internal/modules/knowledge/corpustext.go", "maxAskQuestion", corpusBody("post", "/knowledge/corpora/{id}/ask", "question")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusName", corpusBody("post", "/knowledge/corpora", "name")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusTopic", corpusBody("post", "/knowledge/corpora", "topic_statement")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusDescription", corpusBody("post", "/knowledge/corpora", "description")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusName", corpusBody("patch", "/knowledge/corpora/{id}", "name")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusTopic", corpusBody("patch", "/knowledge/corpora/{id}", "topic_statement")},
	{"internal/modules/knowledge/corpustext.go", "maxCorpusDescription", corpusBody("patch", "/knowledge/corpora/{id}", "description")},
	{"internal/modules/contacts/contacttextbounds.go", "ContactNameLimit", schemaProperty("CreateLeadRequest", "full_name")},
	{"internal/modules/contacts/contacttextbounds.go", "ContactNameLimit", schemaProperty("UpdateLeadRequest", "full_name")},
	{
		"internal/modules/deals/win_evidence.go", "maxWonReasonDetail",
		schemaPath("components", "schemas", "AdvanceDealRequest", "properties", "won_without_contract_detail"),
	},
	{
		"internal/shared/kernel/statedreason/statedreason.go", "Max",
		schemaPath("components", "schemas", "RetentionOverrideRequest", "properties", "reason"),
	},
	{sharedTextLimits, "TextDocumentCharacters", schemaPath("components", "schemas", "CreateCompanyRequest", "properties", "description")},
	{sharedTextLimits, "TextDocumentCharacters", schemaPath("components", "schemas", "UpdateCompanyRequest", "properties", "description")},
	{sharedTextLimits, "TextDocumentCharacters", schemaPath("components", "schemas", "CreateDealRequest", "properties", "description")},
	{sharedTextLimits, "TextDocumentCharacters", schemaPath("components", "schemas", "UpdateDealRequest", "properties", "description")},
}

// sourceBodies are the create bodies whose free-text `source` goes through
// provenance.RefuseSource, which refuses it past SourceIDMax.
var sourceBodies = []string{
	"AddDealRoomDocumentRequest", "CreateActivityRequest",
	"CreateDealRoomRequest", "CreateTaskRequest", "InviteDealRoomParticipantRequest",
	"OpenDealRoomThreadRequest", "PostDealRoomCommentRequest",
}

// emailPaths are the addresses whose writer parses them with values.ParseEmail,
// which refuses one past MaxEmailLength.
var emailPaths = [][]string{
	schemaProperty("ContactEmailInput", "email"), schemaProperty("QuickCaptureContactRequest", "email"),
	schemaProperty("CreateLeadRequest", "email"), schemaProperty("UpdateLeadRequest", "email"),
	schemaProperty("CreateCompanyRequest", "email"), schemaProperty("UpdateCompanyRequest", "email"),
	schemaProperty("InviteDealRoomParticipantRequest", "email"), schemaProperty("UpdateDealRoomParticipantRequest", "email"),
	schemaProperty("InviteUserRequest", "email"),
}

// sharedCapLimits are the caps one shared function applies to many bodies.
func sharedCapLimits() []textLimit {
	var limits []textLimit
	for _, body := range sourceBodies {
		limits = append(limits, textLimit{"internal/shared/kernel/provenance/sourceid.go", "SourceIDMax", schemaProperty(body, "source")})
	}
	for _, path := range emailPaths {
		limits = append(limits, textLimit{"internal/shared/kernel/values/email.go", "MaxEmailLength", path})
	}
	return limits
}

// Create ceilings are unconditional. Replace-set updates admit stored lists
// past those ceilings, which maxItems cannot express.
var createListLimits = []textLimit{
	{"internal/modules/contacts/listcaps.go", "maxContactEmails", schemaProperty("CreateContactRequest", "emails")},
	{"internal/modules/contacts/listcaps.go", "maxContactPhones", schemaProperty("CreateContactRequest", "phones")},
	{"internal/modules/contacts/listcaps.go", "maxCompanyDomains", schemaProperty("CreateCompanyRequest", "domains")},
}

func schemaProperty(schema, property string) []string {
	return schemaPath("components", "schemas", schema, "properties", property)
}

func corpusBody(method, route, property string) []string {
	return []string{"paths", route, method, "requestBody", "content", "application/json", "schema", "properties", property}
}

func TestEachMirroredTextCapIsTheContractsMaxLength(t *testing.T) {
	t.Parallel()
	doc := textLimitsContract(t)
	for _, limit := range slices.Concat(textLimits, sharedCapLimits()) {
		published := publishedBound(t, doc, limit.path, "maxLength")
		if applied := appliedIntConst(t, limit.file, limit.constant); applied != published {
			t.Errorf("%s in %s applies %d and api/crm.yaml publishes maxLength %d at %v — one refuses what the other calls valid",
				limit.constant, limit.file, applied, published, limit.path)
		}
	}
}

func TestEachMirroredListCapIsTheContractsMaxItems(t *testing.T) {
	t.Parallel()
	doc := textLimitsContract(t)
	for _, limit := range createListLimits {
		published := publishedBound(t, doc, limit.path, "maxItems")
		if applied := appliedIntConst(t, limit.file, limit.constant); applied != published {
			t.Errorf("%s applies %d and api/crm.yaml publishes maxItems %d at %v — one refuses what the other calls valid",
				limit.constant, applied, published, limit.path)
		}
	}
}

func TestReplaceSetSchemasAdmitStoredListsPastTheCreateCap(t *testing.T) {
	t.Parallel()
	doc := textLimitsContract(t)
	for _, limit := range createListLimits {
		path := slices.Clone(limit.path)
		path[2] = "Update" + strings.TrimPrefix(path[2], "Create")
		if _, bounded := textLimitSchema(t, doc, path)["maxItems"]; bounded {
			t.Errorf("%v has maxItems: remove the unconditional ceiling so oversized stored lists can shrink", path)
		}
	}
}

func TestEmailReferenceSchemasAdmitStoredAddressesPastTheWriteCap(t *testing.T) {
	t.Parallel()
	doc := textLimitsContract(t)
	for _, path := range [][]string{
		schemaProperty("UpdateMemberRequest", "email"),
		schemaProperty("ContactEmailReplacementInput", "email"),
		schemaProperty("FormerMemberRequest", "email"),
		schemaProperty("DealRoomLinkRequest", "email"),
		corpusBody("post", "/auth/forgot-password", "email"),
	} {
		if _, bounded := textLimitSchema(t, doc, path)["maxLength"]; bounded {
			t.Errorf("%v has maxLength: existing addresses must reach their lookup or unchanged-value check", path)
		}
	}
}

func TestContactEmailReplacementMirrorsTheCreateShape(t *testing.T) {
	t.Parallel()
	doc := textLimitsContract(t)
	create := textLimitSchema(t, doc, schemaPath("components", "schemas", "ContactEmailInput"))
	replace := textLimitSchema(t, doc, schemaPath("components", "schemas", "ContactEmailReplacementInput"))
	if replace["x-go-type"] != "ContactEmailInput" {
		t.Fatal("contact replacements must use the create shape's generated Go mapping")
	}
	delete(replace, "x-go-type")
	delete(create, "description")
	delete(replace, "description")
	delete(textLimitSchema(t, doc, schemaProperty("ContactEmailInput", "email")), "maxLength")
	if !reflect.DeepEqual(create, replace) {
		t.Error("contact email creation and replacement differ beyond the address ceiling")
	}
}

//craft:ignore naked-any JSON Schema validates decoded request bodies
func TestContactAddressContractAdmitsLegacyReplacementAndBoundsCreation(t *testing.T) {
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		t.Fatal(err)
	}
	limit := appliedIntConst(t, "internal/shared/kernel/values/email.go", "MaxEmailLength")
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		path := "/contacts"
		if method == http.MethodPatch {
			path += "/{id}"
		}
		operation := doc.Paths.Value(path).GetOperation(method)
		httpSchema := operation.RequestBody.Value.Content.Get("application/json").Schema.Value
		for _, size := range []int{limit, limit + 1} {
			t.Run(method+"/"+strconv.Itoa(size), func(t *testing.T) {
				address := strings.Repeat("a", size-len("@legacy.test")) + "@legacy.test"
				body := map[string]any{
					"full_name": "Legacy contact",
					"emails":    []any{map[string]any{"email": address}},
				}
				if method == http.MethodPost {
					body["source"] = "manual"
				}
				err := httpSchema.VisitJSON(body)
				refused := method == http.MethodPost && size > limit
				if (err != nil) != refused {
					t.Errorf("%s address length %d: schema error %v, want refused %t", method, size, err, refused)
				}
			})
		}
	}
}

func textLimitsContract(t *testing.T) map[string]any {
	t.Helper()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc map[string]any
	if err := unmarshalYAML(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	return doc
}

func publishedMaxLength(t *testing.T, doc map[string]any, path []string) int {
	t.Helper()
	return publishedBound(t, doc, path, "maxLength")
}

// publishedBound walks the contract to a schema node and answers its bound
// keyword. It fails when the node moved or carries none, because a gate
// whose subject vanished must say so rather than pass over nothing.
func publishedBound(t *testing.T, doc map[string]any, path []string, keyword string) int {
	t.Helper()
	schema := textLimitSchema(t, doc, path)
	bound, ok := schema[keyword].(int)
	if !ok || bound <= 0 {
		t.Fatalf("api/crm.yaml %v publishes no positive %s — retire the cap from this gate or restore the bound", path, keyword)
	}
	return bound
}

func textLimitSchema(t *testing.T, doc map[string]any, path []string) map[string]any {
	t.Helper()
	var node any = doc
	for _, key := range path {
		next, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("api/crm.yaml has no %v — the gate's subject moved; repoint it", path)
		}
		if node, ok = next[key]; !ok {
			t.Fatalf("api/crm.yaml has no %q on the way to %v — the gate's subject moved; repoint it", key, path)
		}
	}
	schema, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("api/crm.yaml %v is not a schema node", path)
	}
	return schema
}

// appliedIntConst reads a package's integer constant from source. The constants
// stay unexported: exporting one to suit a gate would widen the surface to make
// the check convenient.
// sharedTextLimits declares the four shared limits a cap may name instead of a literal.
const sharedTextLimits = "internal/shared/kernel/values/textlimits.go"

func appliedIntConst(t *testing.T, file, name string) int {
	t.Helper()
	parsed, err := gatekit.ParseFile(file, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name {
					continue
				}
				if shared, ok := value.Values[i].(*ast.SelectorExpr); ok && types.ExprString(shared.X) == "values" {
					return appliedIntConst(t, sharedTextLimits, shared.Sel.Name)
				}
				literal, ok := value.Values[i].(*ast.BasicLit)
				if !ok || literal.Kind != token.INT {
					t.Fatalf("%s in %s is neither an integer literal nor a shared values limit — this gate reads only those shapes", name, file)
				}
				text := types.ExprString(literal)
				n, err := strconv.Atoi(text)
				if err != nil {
					t.Fatalf("%s = %q: %v", name, text, err)
				}
				return n
			}
		}
	}
	t.Fatalf("%s declares no %s — the cap the contract publishes is applied by nothing", file, name)
	return 0
}
