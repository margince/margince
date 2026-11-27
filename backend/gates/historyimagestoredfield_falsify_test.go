// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

import (
	"slices"
	"strings"
	"testing"
)

const historyLedgerFixture = `
 type receipt struct{key string}
 type ledgerDB struct{}
 func(ledgerDB) Query(_ context.Context,_ string,_ ...any)ledgerDB{return ledgerDB{}}
 func(ledgerDB) Scan(_ ...any){}
 func(ledgerDB) Exec(_ context.Context,_ string,_ ...any){}
 func writeReceipt(){row:=receipt{key:"ledger_field"};_ = row;ledgerDB{}.Exec(ctx,"INSERT INTO ledger(key, owner) VALUES($1, $2)",row.key,"owner")}
 func readReceipt()receipt{var row receipt;var id int;ledgerDB{}.Query(ctx,"SELECT id, key FROM ledger").Scan(&id,&row.key);return row}
 func w(){_=k.AuditEvent(ctx,tx,"update","contact",1,map[string]any{readReceipt().key:1})}
`

func TestHistoryReadsTheKeyWrittenThroughTheSameLedgerColumn(t *testing.T) {
	t.Parallel()
	if keys := keysOfPlantedWriter(t, historyLedgerFixture); !slices.Contains(keys, "ledger_field") {
		t.Fatalf("lost the persisted key: %v", keys)
	}
}

func TestHistoryRefusesAnUnconstrainedLedgerKey(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		strings.Replace(historyLedgerFixture, `row.key,"owner"`, `"unrelated_insert_key","owner"`, 1),
		strings.Replace(historyLedgerFixture, `SELECT id, key FROM ledger`, `SELECT id, key FROM another_ledger`, 1),
		historyLedgerFixture + `func opaqueWrite(){ledgerDB{}.Exec(ctx,"UPDATE ledger SET owner=$1,key=$2","owner","unbounded")}`,
		historyLedgerFixture + `func opaqueWrite(){ledgerDB{}.Exec(ctx,"INSERT INTO ledger(key,owner) SELECT key,owner FROM elsewhere")}`,
		historyLedgerFixture + `type batch struct{};func(batch)Queue(_ string,_ ...any){};func opaqueWrite(){batch{}.Queue("INSERT INTO ledger(key,owner) VALUES($1,$2)","unbounded","owner")}`,
		historyLedgerFixture + `func sqlf(s string,args ...any)string{return s};func opaqueWrite(){ledgerDB{}.Exec(ctx,sqlf("INSERT INTO ledger(key,owner) VALUES(%s,%s)","unbounded","owner"))}`,
	} {
		census := fixtureCensus(t, fixtureSeed, map[string]string{"a.go": fixtureHeader + body})
		sites, _ := census.doorSites()
		if len(sites) != 1 {
			t.Fatalf("found %d writers", len(sites))
		}
		site := sites[0]
		reader := historyImageReader{index: site.index}
		for _, arg := range site.imageArgs() {
			if keys, known := reader.keys(historyImageArg{expr: arg, pkg: site.use.pkg, decl: site.use.decl}, 0); known {
				t.Fatalf("an unrelated ledger source made a runtime key look closed: %v", keys)
			}
		}
	}
}
