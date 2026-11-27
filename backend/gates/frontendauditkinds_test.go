// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every record type an audit row names has words in the audit log, and the
// log's own map words no type that nothing writes.
//
// The corpus is the writers: auditentitycensus_test.go over the Go, and the
// migrations' INSERTs. An extension's tables stay in its own `ext_` namespace,
// which it words itself.

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

const (
	frontendAuditKinds  = "../frontend/src/features/settings/audit.kinds.ts"
	frontendAuditPage   = "../frontend/src/features/settings/audit.tsx"
	frontendSearchKinds = "../frontend/src/lib/searchkinds.ts"
)

// runtimeTypedAuditWriters names the Go vocabulary each writer draws its record
// type from, where the census cannot read the value. A key names a writer and
// never a type, so the types still come from the Go that owns them.
var runtimeTypedAuditWriters = map[string]func(t *testing.T) []string{
	"backend/internal/modules/identity/grants.go:createRecordGrantInTx":   recordTypeNames,
	"backend/internal/modules/identity/grants.go:RevokeRecordGrant":       recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:moveGrant":           recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:foldGrant":           recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:auditUnshareByMerge": recordTypeNames,
	"backend/internal/modules/recordlinks/write.go:recordLinkChange":      recordTypeNames,
	// The claim route's record type comes off the URL; the store refuses any
	// but a contact, company or lead.
	"backend/internal/contracts/api_gen.go:ClaimRecord": recordTypeNames,
	// A confirmation is about the contact or lead a send is addressed to.
	"backend/internal/modules/consent/lawfulbasiswarning.go:confirmOneTx":       confirmedSubjectKinds,
	"backend/internal/platform/recordmerge/kindid.go:auditDeclaredMoves":        declaredRelinkedEntities,
	"backend/internal/modules/privacy/erasure_declared.go:redactDeclaredTables": declaredErasedEntities,
	"backend/internal/modules/privacy/retentionactions.go:apply":                retentionDispatchObjects,
	// A stored setting's change is audited under the object that gates it.
	"backend/internal/platform/config/spec.go:DefineStored":  settingObjects,
	"backend/internal/platform/config/sealed.go:sealedEntry": settingObjects,
	// These name only rows of an extension's own `ext_` tables.
	"backend/internal/modules/privacy/erasure_extension.go:clearExtensionRows":   extensionTypes,
	"backend/internal/modules/privacy/retention_extension.go:expireExtensionRow": extensionTypes,
	"backend/internal/compose/extledger.go:recordExtensionChange":                extensionTypes,
	// The masking repair re-audits a record whose audit images it read back, so
	// its type is one another writer named first.
	"backend/internal/compose/credentialrepair/credentialrepair.go:maskAuditImagesBatch": noCoreTypes,
}

func recordTypeNames(*testing.T) []string {
	var names []string
	for _, record := range datasource.RecordTypes() {
		names = append(names, string(record))
	}
	return names
}

func declaredRelinkedEntities(*testing.T) []string {
	return declaredEntities(func(table reach.Table) bool { return len(table.Links) > 0 })
}

func declaredErasedEntities(*testing.T) []string {
	return declaredEntities(func(table reach.Table) bool { return len(table.Erase) > 0 })
}

func declaredEntities(keep func(reach.Table) bool) []string {
	var names []string
	for _, owned := range corereach.Tables() {
		if keep(owned.Table) {
			names = append(names, cmp.Or(owned.Table.Entity, owned.Table.Name))
		}
	}
	return names
}

func settingObjects(*testing.T) []string {
	var names []string
	for _, spec := range compose.SettingsCatalogForTest() {
		names = append(names, spec.Object)
	}
	return names
}

// retentionDispatchObjects reads the object of each pair the retention pass
// dispatches, from the keys of its action table.
func retentionDispatchObjects(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, pair := range auditPackageMapKeys(t, modulePath+"/internal/modules/privacy", "retentionActions") {
		object, _, _ := strings.Cut(pair, "/")
		names = append(names, object)
	}
	return names
}

func noCoreTypes(*testing.T) []string { return nil }

func extensionTypes(*testing.T) []string { return []string{extensionRecordTypes} }

// confirmedSubjectKinds reads the kinds requireSubjectVisible admits, from the
// cases of its switch. A confirmation of any other kind is refused there.
func confirmedSubjectKinds(t *testing.T) []string {
	t.Helper()
	var kinds []string
	for _, pkg := range typedModule(t) {
		if pkg.path != modulePath+"/internal/modules/consent" {
			continue
		}
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != "requireSubjectVisible" {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if clause, isCase := n.(*ast.CaseClause); isCase {
						for _, expr := range clause.List {
							if value, isConst := auditConstString(pkg.info, expr); isConst {
								kinds = append(kinds, value)
							}
						}
					}
					return true
				})
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatal("consent's requireSubjectVisible admits no constant kind, the vocabulary this gate reads for confirmOneTx")
	}
	return kinds
}

// auditPackageMapKeys reads the constant keys of a package-level map literal.
func auditPackageMapKeys(t *testing.T, pkgPath, name string) []string {
	t.Helper()
	for _, pkg := range typedModule(t) {
		if pkg.path != pkgPath {
			continue
		}
		for _, file := range pkg.files {
			for ident, obj := range pkg.info.Defs {
				if obj == nil || ident.Name != name || obj.Parent() != obj.Pkg().Scope() {
					continue
				}
				if keys, ok := auditMapLiteralKeys(pkg, file, ident); ok {
					return keys
				}
			}
		}
	}
	t.Fatalf("%s declares no map literal named %s, which this gate reads", pkgPath, name)
	return nil
}

func auditMapLiteralKeys(pkg typedPackage, file *ast.File, ident *ast.Ident) ([]string, bool) {
	var keys []string
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0] != ident || len(spec.Values) != 1 {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			return false
		}
		found = true
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if tv := pkg.info.Types[kv.Key]; tv.Value != nil && tv.Value.Kind() == constant.String {
				keys = append(keys, constant.StringVal(tv.Value))
			}
		}
		return false
	})
	return keys, found
}

// goAuditedRecordTypes are the record types Go code audits: each constant a
// writer is called with, and each vocabulary a runtime-typed writer draws
// from. unread are the calls whose type neither names.
func goAuditedRecordTypes(t *testing.T, census *auditEntityCensus) (written, named map[string]bool, unread []auditEntitySite) {
	t.Helper()
	written, named = map[string]bool{}, map[string]bool{}
	for _, site := range census.sites {
		if site.opaque == "" {
			written[site.value] = true
			continue
		}
		vocabulary, known := runtimeTypedAuditWriters[site.writer]
		if !known {
			unread = append(unread, site)
			continue
		}
		named[site.writer] = true
		for _, value := range vocabulary(t) {
			written[value] = true
		}
	}
	return written, named, unread
}

func TestEveryAuditedRecordTypeHasWordsInTheAuditLog(t *testing.T) {
	t.Parallel()
	census := auditEntityCensusOf(t)
	if !census.seedInserts {
		t.Fatalf("%s no longer holds the INSERT into audit_log with a %s parameter, so the census starts from nothing", auditSeedFunc, auditSeedParam)
	}
	written, named, unread := goAuditedRecordTypes(t, census)
	for _, site := range unread {
		t.Errorf("%s writes an audit row whose record type is %s, which this gate cannot read.\n"+
			"\tPass a constant, or name the Go vocabulary that type is drawn from in runtimeTypedAuditWriters, keyed %q.",
			site.pos, site.opaque, site.writer)
	}
	for key := range runtimeTypedAuditWriters {
		if !named[key] {
			t.Errorf("runtimeTypedAuditWriters names %s, whose audit call no longer takes a record type this gate cannot read", key)
		}
	}
	for _, at := range census.asValue {
		t.Errorf("%s hands an audit writer on as a value, so the calls made through it are not read: call it directly", at)
	}
	for _, key := range census.unreadInterfaces {
		t.Errorf("%s shares a writer's name and signature, and no module type is read to reach the writer through it: "+
			"implement it with a module type, or rename the method", key)
	}
	for _, at := range census.directInserts {
		t.Errorf("%s sends its own INSERT into audit_log, which this gate does not follow: call a storekit writer", at)
	}
	migrated, opaque := auditMigrationTypes(t, filepath.Join(gatekit.ModuleRoot(t), "migrations"))
	if len(migrated) == 0 {
		t.Error("read no audit row out of the migrations, so their INSERTs are no longer seen")
	}
	for _, path := range opaque {
		t.Errorf("%s inserts an audit row whose entity_type this gate cannot read: "+
			"name the INSERT's columns and spell entity_type as literals", path)
	}
	for value := range migrated {
		written[value] = true
	}
	for value := range written {
		if strings.HasPrefix(value, "ext_") {
			delete(written, value)
		}
	}
	worded, owned, problems := auditKindLabels(t, frontendAuditPage, frontendAuditKinds, frontendSearchKinds)
	for _, problem := range problems {
		t.Error(problem)
	}
	if len(written) == 0 || len(owned) == 0 {
		t.Fatalf("read %d audited types and %d owned labels, and an empty side agrees with everything", len(written), len(owned))
	}
	for _, value := range sortedKeys(written) {
		if !worded[value] {
			t.Errorf("audit rows name record type %q and the audit log has no words for it: add it to AUDIT_KIND_LABEL in %s", value, frontendAuditKinds)
		}
	}
	for _, key := range sortedKeys(owned) {
		if !written[key] {
			t.Errorf("the audit log words record type %q, which no audit row names: remove it from %s", key, frontendAuditKinds)
		}
	}
}

// auditKindLabels reads KIND_LABELS in audit.tsx: the types it has words for,
// and the ones whose words it owns. It spreads the search kinds, which search
// owns, so those count as words but never as stale.
func auditKindLabels(t *testing.T, page, kinds, search string) (worded, owned map[string]bool, problems []string) {
	t.Helper()
	source, err := gatekit.ReadFile(page)
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}
	text := gatekit.TSWithoutComments(string(source))
	at := indexAfter(text, "const KIND_LABELS")
	open := -1
	if at >= 0 {
		open = strings.Index(text[at:], "([")
	}
	if open < 0 {
		t.Fatalf("%s no longer builds KIND_LABELS as new Map([...]), the shape this gate reads", page)
	}
	body, ok := tsBalanced(text[at+open+1:])
	if !ok {
		t.Fatalf("%s: KIND_LABELS is unterminated", page)
	}
	worded, owned = map[string]bool{}, map[string]bool{}
	for _, item := range tsTopLevel(body) {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
		case item == "...Object.entries(AUDIT_KIND_LABEL)":
			for key := range tsKeySet(t, tsObjectEntries(t, kinds, "export const AUDIT_KIND_LABEL")) {
				worded[key], owned[key] = true, true
			}
		case item == "...Object.entries(SEARCH_HIT_KIND_KEY)":
			for key := range tsKeySet(t, tsObjectEntries(t, search, "export const SEARCH_HIT_KIND_KEY")) {
				worded[key] = true
			}
		case strings.HasPrefix(item, "["):
			pair := tsTopLevel(strings.Trim(item, "[]"))
			key := strings.Trim(strings.TrimSpace(pair[0]), `"'`)
			worded[key], owned[key] = true, true
		default:
			problems = append(problems, fmt.Sprintf("KIND_LABELS in %s holds %q, which this gate does not read", page, item))
		}
	}
	return worded, owned, problems
}
