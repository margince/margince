// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every record type an audit row names has words in the audit log, and the
// log's own map words no type that nothing writes.
//
// The corpus is the writers: auditwrites_test.go and the migrations' INSERTs.
// An extension's tables stay in its own `ext_` namespace, which it words itself.

import (
	"cmp"
	"strings"
	"testing"

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

// runtimeTypedAuditWriters names, for each function whose audit call takes its
// record type at runtime, the Go vocabulary that type is drawn from. A key names
// a writer and never a type, so the types still come from the Go that owns them.
var runtimeTypedAuditWriters = map[string]func(t *testing.T) []string{
	"backend/internal/modules/identity/grants.go:createRecordGrantInTx":   recordTypeNames,
	"backend/internal/modules/identity/grants.go:RevokeRecordGrant":       recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:moveGrant":           recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:foldGrant":           recordTypeNames,
	"backend/internal/modules/identity/grantmerge.go:auditUnshareByMerge": recordTypeNames,
	"backend/internal/modules/recordlinks/write.go:recordLinkChange":      recordTypeNames,
	// A confirmation is about the contact or lead a send is addressed to.
	"backend/internal/modules/consent/lawfulbasiswarning.go:confirmOneTx":       recordTypeNames,
	"backend/internal/modules/contacts/mergekindid.go:auditDeclaredMoves":       declaredRelinkedEntities,
	"backend/internal/modules/privacy/erasure_declared.go:redactDeclaredTables": declaredErasedEntities,
	"backend/internal/modules/privacy/retentionactions.go:apply":                retentionDispatchObjects,
	// These name only rows of an extension's own `ext_` tables.
	"backend/internal/modules/privacy/erasure_extension.go:clearExtensionRows":   noCoreTypes,
	"backend/internal/modules/privacy/retention_extension.go:expireExtensionRow": noCoreTypes,
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

// retentionDispatchObjects reads the object of each pair the retention pass
// dispatches, from the keys of its action table.
func retentionDispatchObjects(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, pair := range auditPackageMapKeys(t, "backend/internal/modules/privacy", "retentionActions") {
		object, _, _ := strings.Cut(pair, "/")
		names = append(names, object)
	}
	return names
}

func noCoreTypes(*testing.T) []string { return nil }

func TestEveryAuditedRecordTypeHasWordsInTheAuditLog(t *testing.T) {
	t.Parallel()
	written := map[string]bool{}
	named := map[string]bool{}
	for _, site := range auditWriteSites(t) {
		for _, value := range site.types {
			written[value] = true
		}
		if site.opaque == "" {
			continue
		}
		if site.asValue {
			t.Errorf("%s hands the audit writer %s on as a value, so the calls made through it are not read: call it directly",
				site.pos, site.opaque)
			continue
		}
		key := site.pos[:strings.LastIndex(site.pos, ":")] + ":" + site.fn
		vocabulary, known := runtimeTypedAuditWriters[key]
		if !known {
			t.Errorf("%s writes an audit row whose record type is %s, read at runtime.\n"+
				"\tName the Go vocabulary that type is drawn from in runtimeTypedAuditWriters, keyed %q.",
				site.pos, site.opaque, key)
			continue
		}
		named[key] = true
		for _, value := range vocabulary(t) {
			written[value] = true
		}
	}
	for key := range runtimeTypedAuditWriters {
		if !named[key] {
			t.Errorf("runtimeTypedAuditWriters names %s, whose audit call no longer takes its record type at runtime", key)
		}
	}
	direct, seedSeen := directAuditInserts(t)
	for _, at := range direct {
		t.Errorf("%s sends its own INSERT into audit_log, which this census does not follow: call a storekit door", at)
	}
	if !seedSeen {
		t.Errorf("no INSERT into audit_log was found in storekit's %s, so the direct-INSERT check has stopped seeing it", auditSeed)
	}
	migrated, opaque := auditMigrationTypes(t)
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
	worded, owned := auditKindLabels(t)
	if len(written) == 0 || len(owned) == 0 {
		t.Fatalf("read %d audited types and %d owned labels, and an empty side agrees with everything", len(written), len(owned))
	}
	for _, value := range sortedKeys(written) {
		if !worded[value] {
			t.Errorf("audit rows name record type %q and the audit log has no words for it", value)
		}
	}
	for _, key := range sortedKeys(owned) {
		if !written[key] {
			t.Errorf("the audit log words record type %q, which no audit row names", key)
		}
	}
}

// auditKindLabels reads KIND_LABELS in audit.tsx: the types it has words for,
// and the ones whose words it owns. It spreads the search kinds, which search
// owns, so those count as words but never as stale.
func auditKindLabels(t *testing.T) (worded, owned map[string]bool) {
	t.Helper()
	source, err := gatekit.ReadFile(frontendAuditPage)
	if err != nil {
		t.Fatalf("reading %s: %v", frontendAuditPage, err)
	}
	text := tsWithoutComments(string(source))
	at := indexAfter(text, "const KIND_LABELS")
	open := -1
	if at >= 0 {
		open = strings.Index(text[at:], "([")
	}
	if open < 0 {
		t.Fatalf("%s no longer builds KIND_LABELS as new Map([...]), the shape this gate reads", frontendAuditPage)
	}
	body, ok := tsBalanced(text[at+open+1:])
	if !ok {
		t.Fatalf("%s: KIND_LABELS is unterminated", frontendAuditPage)
	}
	worded, owned = map[string]bool{}, map[string]bool{}
	for _, item := range tsTopLevel(body) {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
		case item == "...Object.entries(AUDIT_KIND_LABEL)":
			for key := range tsKeySet(t, tsObjectEntries(t, frontendAuditKinds, "export const AUDIT_KIND_LABEL")) {
				worded[key], owned[key] = true, true
			}
		case item == "...Object.entries(SEARCH_HIT_KIND_KEY)":
			for key := range tsKeySet(t, tsObjectEntries(t, frontendSearchKinds, "export const SEARCH_HIT_KIND_KEY")) {
				worded[key] = true
			}
		case strings.HasPrefix(item, "["):
			pair := tsTopLevel(strings.Trim(item, "[]"))
			key := strings.Trim(strings.TrimSpace(pair[0]), `"'`)
			worded[key], owned[key] = true, true
		default:
			t.Errorf("KIND_LABELS in %s holds %q, which this gate does not read", frontendAuditPage, item)
		}
	}
	return worded, owned
}
