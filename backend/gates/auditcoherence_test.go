// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The audit_log actor_type vocabulary in crm.yaml matches the table's CHECK
// constraint. The effective set is the highest-numbered migration that
// re-states it. When the two drift, the DB rejects an actor the contract
// defines at write time. Only a real INSERT surfaces that audit-integrity bug.
//
// The action column is bound in enumsync_test.go to contracts.AuditAction,
// which is generated from crm.yaml. actor_type is an inline enum with no
// generated type of its own, so this test is its coherence floor.
//
// The contract is read by text scan, not a YAML library: the root
// fitness-test package stays free of parser deps (the arch-lint boundary),
// the same reason enumsync_test.go derives the DDL sets by regex.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestAuditLogEnumCoherence(t *testing.T) {
	t.Parallel()
	contract := auditLogContractActorType(t)
	const column = "audit_log.actor_type"
	got, ok := checkVocabularies(t)[column]
	if !ok {
		t.Fatalf("%s: no CHECK vocabulary found in the head catalog", column)
	}
	ddlSet, conSet := auditStrSet(got), auditStrSet(contract)
	// Every contract value must be a legal DB value, else code that emits it
	// would 500 at INSERT.
	for _, v := range contract {
		if _, ok := ddlSet[v]; !ok {
			t.Errorf("%s: contract defines %q but the DDL CHECK rejects it — a write of this actor would 500; widen the CHECK in a new migration to match crm.yaml", column, v)
		}
	}
	for _, v := range got {
		if _, ok := conSet[v]; !ok {
			t.Errorf("%s: DDL CHECK allows %q but the contract does not define it — add it to crm.yaml (P3)", column, v)
		}
	}
}

// The third copy of the vocabulary is the SOURCE. Every storekit.Audit call
// passes its verb as a string literal, and neither check above can see one: a
// verb no list contains leaves the contract and the DDL in perfect agreement
// and still fails at INSERT, taking its whole transaction with it.
//
// That is not hypothetical. `connect` and `disconnect` shipped this way and
// made binding a data provider impossible — the CHECK rejected the audit row,
// the transaction rolled back, and the admin was told to check their own input
// (migration 0226). Unit tests could not catch it because the constraint lives
// only in Postgres, and the integration tests seeded rows directly.
//
// So this reads the tree the way the other direction reads the migrations.
func TestEveryAuditVerbTheCodeWritesIsLegal(t *testing.T) {
	t.Parallel()
	ddl := checkVocabularies(t)
	allowed, ok := ddl["audit_log.action"]
	if !ok {
		t.Fatal("audit_log.action: no CHECK vocabulary found in the head catalog")
	}
	allowedSet := auditStrSet(allowed)

	// The first string argument after the transaction is the verb. A call that
	// builds it from a variable is invisible here, which is why the write shape
	// spells them as literals.
	call := regexp.MustCompile(`storekit\.Audit(?:Event)?(?:WithEvidence)?\(\s*[A-Za-z0-9_.]+\s*,\s*[A-Za-z0-9_.]+\s*,\s*"([a-z_]+)"`)

	found := map[string][]string{}
	for _, dir := range []string{"internal", "../extensions"} {
		for _, path := range goSourceFiles(t, dir) {
			src, err := gatekit.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, m := range call.FindAllStringSubmatch(string(src), -1) {
				found[m[1]] = append(found[m[1]], path)
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no storekit.Audit call sites found — this gate would pass vacuously; check the call pattern")
	}

	verbs := make([]string, 0, len(found))
	for v := range found {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	for _, v := range verbs {
		if _, ok := allowedSet[v]; !ok {
			t.Errorf("audit_log.action: code writes %q (%s) but the DDL CHECK rejects it — every write of it fails at INSERT and rolls back its whole transaction; add the verb in a new migration and to crm.yaml",
				v, strings.Join(found[v], ", "))
		}
	}
}

// auditLogContractActorType returns the audit log's sorted actor_type enum.
// AuditLogEntry and AuditHistoryEntry must both $ref AuditAction.
func auditLogContractActorType(t *testing.T) []string {
	t.Helper()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	doc := string(raw)
	block := schemaBlock(t, doc, "AuditLogEntry")
	actionRef := regexp.MustCompile(`(?m)^        action:\s*\n\s+\$ref: '#/components/schemas/AuditAction'`)
	for _, reader := range []string{"AuditLogEntry", "AuditHistoryEntry"} {
		if !actionRef.MatchString(schemaBlock(t, doc, reader)) {
			t.Errorf("contract: %s.action does not $ref AuditAction; a second list of audit verbs "+
				"drifts from the one the CHECK is held to", reader)
		}
	}
	// actor_type: an inline flow-mapping enum on the actor_type line.
	tm := regexp.MustCompile(`actor_type:[^\n]*enum:\s*\[([^\]]*)\]`).FindStringSubmatch(block)
	if tm == nil {
		t.Fatal("contract: AuditLogEntry.actor_type enum not found")
	}
	return splitEnumList(tm[1])
}

// schemaBlock returns the text of a components.schemas.<name> block: from its
// 4-space key line to the next 4-space schema key.
func schemaBlock(t *testing.T, doc, name string) string {
	t.Helper()
	start := regexp.MustCompile(`(?m)^    ` + regexp.QuoteMeta(name) + `:[ \t]*$`).FindStringIndex(doc)
	if start == nil {
		t.Fatalf("contract: schema %s not found", name)
	}
	rest := doc[start[1]:]
	if end := regexp.MustCompile(`(?m)^    [A-Za-z]`).FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

// splitEnumList turns an `a, b, c` flow-list body into a sorted set slice.
func splitEnumList(list string) []string {
	var out []string
	for tok := range strings.SplitSeq(list, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	sort.Strings(out)
	return out
}

func auditStrSet(vals []string) map[string]struct{} {
	m := make(map[string]struct{}, len(vals))
	for _, v := range vals {
		m[v] = struct{}{}
	}
	return m
}
