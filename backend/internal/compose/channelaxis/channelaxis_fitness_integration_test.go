// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channelaxis_test

// The axis separation, held as obligations DERIVED from the system rather than
// as lists somebody has to remember to update (repo rule 2). Each of these
// fails the moment the two vocabularies start leaking into each other again,
// which is the failure ADR-0107/A158 exists to prevent and the one no unit test
// can see: it lives in the schema, the contract document, and the registry at
// once.

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// contractEnumRe finds enum members spelled anywhere in the contract document.
// Deliberately a scan of the raw text rather than a parse of the four nodes
// anyone currently remembers: a provider name reappearing in a FIFTH enum added
// later is exactly the regression this guards, and a parser aimed at the known
// nodes would not see it.
//
// BOTH YAML sequence styles, because the document uses both: flow
// (`enum: [a, b]`) and block (`enum:` then `- a`). Matching only flow would
// leave four existing block-style enums unchecked and the guard would report
// clean over a document it had not fully read.
var (
	contractEnumRe      = regexp.MustCompile(`enum:\s*\[([^\]]*)\]`)
	contractBlockEnumRe = regexp.MustCompile(`(?m)^\s*enum:\s*\n((?:\s*-\s*[^\n]*\n)+)`)
)

// contractEnums returns every enum's member set, from both spellings.
func contractEnums(raw string) [][]string {
	var out [][]string
	for _, m := range contractEnumRe.FindAllStringSubmatch(raw, -1) {
		out = append(out, strings.Split(m[1], ","))
	}
	for _, m := range contractBlockEnumRe.FindAllStringSubmatch(raw, -1) {
		var members []string
		for line := range strings.SplitSeq(m[1], "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "-"); ok {
				members = append(members, strings.TrimSpace(after))
			}
		}
		out = append(out, members)
	}
	return out
}

func contractPath(t *testing.T, name string) string {
	t.Helper()
	// The test binary runs in its package directory; the contract lives at the
	// backend module root.
	return filepath.Join("..", "..", "..", "api", name)
}

// providerCheckLiteralRe reads the names a provider CHECK admits from its
// definition, which spells each as 'name'::text.
var providerCheckLiteralRe = regexp.MustCompile(`'([a-z0-9_]+)'::text`)

// botBindingProviderSets reads the provider sets the schema compiles into the
// two bot-binding tables. An enum is exempt only when it equals one of them.
// So only a bot connector added to the schema widens the exception.
func botBindingProviderSets(ctx context.Context, t *testing.T, owner *pgx.Conn) map[string]bool {
	t.Helper()
	rows, err := owner.Query(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE contype = 'c' AND conrelid IN ('channel_connection'::regclass, 'chat_link'::regclass)
		  AND conname LIKE '%\_provider\_check'`)
	if err != nil {
		t.Fatalf("reading the bot-binding provider checks: %v", err)
	}
	defs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("scanning a bot-binding provider check: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("found %d bot-binding provider checks, want one each on channel_connection and chat_link", len(defs))
	}
	sets := make(map[string]bool, len(defs))
	for _, def := range defs {
		members := map[string]bool{}
		for _, m := range providerCheckLiteralRe.FindAllStringSubmatch(def, -1) {
			members[m[1]] = true
		}
		if len(members) == 0 {
			t.Fatalf("the provider check %q names no provider, so the exception would read nothing", def)
		}
		sets[enumKey(members)] = true
	}
	return sets
}

// enumKey spells a member set in one order, so two sets compare as strings.
func enumKey(members map[string]bool) string {
	return strings.Join(slices.Sorted(maps.Keys(members)), ",")
}

// A registered transport must not appear as a member of any kind-style enum in
// the contract. Derived from channel_provider — the registry IS the list, so a
// provider registered tomorrow is covered without touching this test.
//
// The one exception is an enum of the core bot connectors a workspace binds
// (ChannelConnection, ConnectChannelRequest, ChannelLink). An extension unit
// never writes that surface (ADR-0107).
func TestNoContractEnumSpellsARegisteredTransport(t *testing.T) {
	integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	rows, err := owner.Query(ctx, `SELECT provider FROM channel_provider ORDER BY provider`)
	if err != nil {
		t.Fatalf("reading the provider registry: %v", err)
	}
	var providers []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scanning a provider: %v", err)
		}
		providers = append(providers, p)
	}
	rows.Close()
	if len(providers) == 0 {
		t.Fatal("the provider registry is empty, so this test would pass by having nothing to check")
	}

	raw, err := os.ReadFile(contractPath(t, "crm.yaml"))
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}

	allowed := botBindingProviderSets(ctx, t, owner)

	for _, members := range contractEnums(string(raw)) {
		trimmed := make(map[string]bool, len(members))
		for _, m := range members {
			trimmed[strings.Trim(strings.TrimSpace(m), `'"`)] = true
		}
		if allowed[enumKey(trimmed)] {
			continue
		}
		for _, provider := range providers {
			if trimmed[provider] {
				t.Errorf("a contract enum %v spells the registered transport %q; a provider vocabulary is a deployment fact and cannot live in an installation-independent enum (ADR-0107/A158) — use ProviderRef",
					members, provider)
			}
		}
	}
}

// activity_kind holds EXACTLY the contract's kind vocabulary. One statement
// binding the table to the document, so the two cannot drift into a state where
// a contract-valid kind fails an FK it has no way to know about — the failure
// mode 0240's own seed comment describes — or where the table carries a kind
// nothing can be written with.
//
// The contract side is read through the GENERATED Valid(), not by scraping the
// YAML: the generator already decided what the enum is, a text scan cannot tell
// Activity.kind from an unrelated `kind` enum elsewhere in the document (the
// legal-page kinds are home/impressum/team), and a test that has to be taught
// which node to look at is a list again.
func TestActivityKindTableMatchesTheContractKindEnum(t *testing.T) {
	integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	rows, err := owner.Query(ctx, `SELECT kind FROM activity_kind ORDER BY kind`)
	if err != nil {
		t.Fatalf("reading activity_kind: %v", err)
	}
	var inTable []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scanning a kind: %v", err)
		}
		inTable = append(inTable, k)
	}
	rows.Close()
	if len(inTable) == 0 {
		t.Fatal("activity_kind is empty, so this test would pass by having nothing to check")
	}

	// Every kind the table carries must be one the contract admits.
	for _, k := range inTable {
		if !crmcontracts.ActivityKind(k).Valid() {
			t.Errorf("activity_kind carries %q and the contract does not admit it: a kind nothing can be written with is a vocabulary nobody maintains", k)
		}
	}

	// And the reverse, which is the direction that breaks a caller: every kind
	// the contract admits must exist in the table, or a contract-valid request
	// fails a foreign key the caller cannot see.
	//
	// This half IS a hand-written list, unlike everything else in this file: the
	// generator emits no All-slice to walk, and Valid() answers per value rather
	// than enumerating. A seventh kind added to the contract without a row here
	// would pass — stated rather than glossed, because a test that looks derived
	// and is not is worse than one that admits it.
	seeded := make(map[string]bool, len(inTable))
	for _, k := range inTable {
		seeded[k] = true
	}
	for _, k := range []crmcontracts.ActivityKind{
		crmcontracts.ActivityKindCall,
		crmcontracts.ActivityKindEmail,
		crmcontracts.ActivityKindMeeting,
		crmcontracts.ActivityKindMessage,
		crmcontracts.ActivityKindNote,
		crmcontracts.ActivityKindTask,
	} {
		if !seeded[string(k)] {
			t.Errorf("the contract admits kind %q and activity_kind does not carry it: a contract-valid request would fail a foreign key the caller cannot see", k)
		}
	}
}
