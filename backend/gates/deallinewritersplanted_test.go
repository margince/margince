// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

// A census must reject new writers and misleading guard paths, even when
// they sit outside the files that introduced the line-value engine.

package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func lineFixture(t *testing.T, sources ...string) *lineLockFlow {
	t.Helper()
	var files []*ast.File
	for i, source := range sources {
		prefix := "package fixture\nimport sk \"github.com/margince/margince/backend/internal/platform/database/storekit\"\nimport \"fmt\"\nconst modeLines=\"lines\"\n"
		file, err := parser.ParseFile(gatekit.SourceFileSet(), fmt.Sprintf("%s/part%d.go", t.TempDir(), i), prefix+source, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return lineLocksOf(files)
}

func TestLineMoneyCensusRefusesPlantedPatchBypasses(t *testing.T) {
	t.Parallel()
	const deal = `l,_:=sk.LockRow(ctx,tx,"deal",id,live);`
	const offer = `l,_:=sk.LockRow(ctx,tx,"offer",id,live);`
	const apply = `p.ApplyLocked(ctx,tx,l)`
	const guard = `if err:=refuseTypedMoneyOnLines(ctx,tx,convert(l.ID()),p);err!=nil{return err};`
	cases := []struct {
		name, source string
		refused      bool
	}{
		{"direct locked write", `func write(){` + deal + apply + `}`, true},
		{"unrelated table", `func write(){` + offer + apply + `}`, false},
		{"held guard", `func write(){` + deal + guard + apply + `}`, false},
		{"guard outside deferred writer", `func write(){` + deal + guard + `register(func(){` + apply + `})}`, true},
		{"patch changed after guard", `func write(){` + deal + guard + `p.Set("amount_minor",nil,amount);` + apply + `}`, true},
		{"lock shadowed after guard", `func write(){` + deal + guard + `{l:=other;` + apply + `}}`, true},
		{"shadowed money guard", `func write(){` + deal + `refuseTypedMoneyOnLines:=func(){return nil};` + guard + apply + `}`, true},
		{"method value patch", `func write(){` + deal + `apply:=p.ApplyLocked;apply(ctx,tx,l)}`, true},
		{"package function literal", `var write=func(){` + deal + apply + `}`, true},
		{"unresolved method caller", `func(s *Store) write(l sk.RowLock){` + apply + `};func(s *Store) safe(){` + offer + `s.write(l)};func unsafe(s *Store){` + deal + `s.write(l)}`, true},
		{"guard after write", `func write(){` + deal + apply + `;` + guard + `}`, true},
		{"guard in another branch", `func write(){` + deal + `if branch{` + guard + `};` + apply + `}`, true},
		{"guard in an unused closure", `func write(){` + deal + `unused:=func(){` + guard + `};` + apply + `}`, true},
		{"different patch", `func write(){` + deal + `if err:=refuseTypedMoneyOnLines(ctx,tx,convert(l.ID()),other);err!=nil{return err};` + apply + `}`, true},
		{"different row", `func write(){` + deal + `if err:=refuseTypedMoneyOnLines(ctx,tx,convert(other.ID()),p);err!=nil{return err};` + apply + `}`, true},
		{"pointer mutation", `func write(){` + offer + `open:=Open{Lock:l};ptr:=&open;ptr.Lock=takeDeal();p.ApplyLocked(ctx,tx,open.Lock)}`, true},
		{"lock alias", `func write(){` + deal + `other:=l;p.ApplyLocked(ctx,tx,other)}`, true},
		{"returned lock", `func take(){` + deal + `return l};func write(){l:=take();` + apply + `}`, true},
		{"returned field", `func take(){` + deal + `return Open{Lock:l}};func write(){open:=take();p.ApplyLocked(ctx,tx,open.Lock)}`, true},
		{"unguarded helper", `func write(l sk.RowLock){` + apply + `};func entry(){` + deal + `write(l)}`, true},
		{"a safe caller cannot hide a second caller", `func write(l sk.RowLock){` + apply + `};func safe(){` + offer + `write(l)};func unsafe(){` + deal + `write(l)}`, true},
		{"exported helper", `func Write(l sk.RowLock){` + apply + `};func entry(){` + offer + `Write(l)}`, true},
		{"escaped helper", `func write(l sk.RowLock){` + apply + `};func entry(){` + offer + `write(l);escaped:=write;register(escaped)}`, true},
		{"shadowed lock factory", `func take(){` + offer + `return l};func write(){take:=func(){` + deal + `return l};l:=take();` + apply + `}`, true},
		{"shadowed storekit import", `func write(){sk:=factory;` + offer + apply + `}`, true},
		{"parameter shadows a constant", `const table="offer";func write(table string){p.ApplyGuarded(ctx,tx,table,id,nil)};func entry(){write("deal")}`, true},
		{"direct guarded API", `func write(){p.ApplyGuarded(ctx,tx,"deal",id,nil)}`, true},
		{"a changed file cannot hide a writer in a second file", `func entry(){helper()}`, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sources := []string{test.source}
			if test.name == "a changed file cannot hide a writer in a second file" {
				sources = append(sources, `func helper(){`+deal+apply+`}`)
			}
			flow := lineFixture(t, sources...)
			refused := false
			for _, site := range flow.dealPatchSites("deal") {
				refused = refused || !flow.patchGuarded(site, lineValueSeam)
			}
			if refused != test.refused {
				t.Fatalf("refused=%v, expected %v", refused, test.refused)
			}
		})
	}
}

func TestLineProductCensusRefusesPlantedGuardBypasses(t *testing.T) {
	t.Parallel()
	const sql = `tx.Exec(ctx,"DELETE FROM deal_product WHERE deal_id=@id",id)`
	const mode = `head,err:=readDealCommercialHead(ctx,tx,id);if err!=nil{return nil,err};if head.Mode==modeLines{return nil,&LineOwnedValueError{}};`
	const check = `if err:=check();err!=nil{return err};`
	cases := []struct {
		name, source string
		guarded      bool
	}{
		{"bare writer", `func write(){` + sql + `}`, false},
		{"mode refusal before write", `func write(){` + mode + sql + `}`, true},
		{"guard checks another deal", `func write(){head,err:=readDealCommercialHead(ctx,tx,other);if err!=nil{return nil,err};if head.Mode==modeLines{return nil,&LineOwnedValueError{}};` + sql + `}`, false},
		{"mode refusal after write", `func write(){` + sql + `;` + mode + `}`, false},
		{"a condition that also needs another fact", `func write(){head,_:=readDealCommercialHead();if head.Mode==modeLines&&manual{return nil,nil};` + sql + `}`, false},
		{"a made-up head", `func write(){head:=Head{};if head.Mode==modeLines{return nil,nil};` + sql + `}`, false},
		{"guarded parent", `func check(){` + mode + `return nil};func entry(){` + check + `write()};func write(){` + sql + `}`, true},
		{"a second unguarded parent", `func check(){` + mode + `return nil};func entry(){` + check + `write()};func other(){write()};func write(){` + sql + `}`, false},
		{"a helper that can succeed before checking", `func check(){if skip{return nil};` + mode + `return nil};func write(){` + check + sql + `}`, false},
		{"a helper that returns success on a lines deal", `func check(){head,_:=readDealCommercialHead();if head.Mode==modeLines{return nil,nil};return nil};func write(){` + check + sql + `}`, false},
		{"guard in an earlier callback", `func check(){` + mode + `return nil};func write(){run(func(){` + check + `});` + sql + `}`, false},
		{"exported writer cannot borrow a parent's check", `func check(){` + mode + `return nil};func entry(){` + check + `Write()};func Write(){` + sql + `}`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			flow := lineFixture(t, test.source)
			name := "write"
			if flow.functions[name] == nil {
				name = "Write"
			}
			var pos token.Pos
			ast.Inspect(flow.functions[name].Body, func(node ast.Node) bool {
				text, ok := node.(*ast.BasicLit)
				if !ok {
					return true
				}
				literal, decoded := gatekit.LiteralText(text)
				if decoded && strings.Contains(literal, "DELETE FROM deal_product") {
					pos = text.Pos()
				}
				return true
			})
			if pos == 0 {
				t.Fatal("the planted product writer was not discovered")
			}
			if got := flow.productsGuarded(name, pos, lineValueSeam, map[string]bool{}); got != test.guarded {
				t.Fatalf("guarded=%v, expected %v", got, test.guarded)
			}
		})
	}
}

func TestLineSettlementSeamHasOnlyItsClosedCallers(t *testing.T) {
	t.Parallel()
	base := `func applyDealLineValueLocked(){};func(s *Store) settleDealLines(){applyDealLineValueLocked()};func(s *Store) leaveLinesMode(){applyDealLineValueLocked()};`
	constants := map[string]string{"commercialModeColumn": "commercial_mode", "commercialRevisionColumn": "commercial_revision"}
	for _, test := range []struct {
		name, source string
		refused      bool
	}{
		{"settlement only", base, false},
		{"another caller", base + `func bypass(){applyDealLineValueLocked()}`, true},
		{"escaped seam", base + `func escape(){register(applyDealLineValueLocked)}`, true},
		{"mode setter", base + `func change(){p.Set("commercial_mode",old,next)}`, true},
		{"revision setter", base + `func change(){p.Set("commercial_revision",old,next)}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			problems := lineSeamProblems(lineFixture(t, test.source), constants)
			if (len(problems) > 0) != test.refused {
				t.Fatalf("findings=%v expected refusal=%v", problems, test.refused)
			}
		})
	}
}

func TestProductPatchesCannotBypassTheProjection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, source string
		refused      bool
	}{
		{"direct product patch", `func write(){p.ApplyGuarded(ctx,tx,"deal_product",id,nil)}`, true},
		{"locked product patch", `func write(){l,_:=sk.LockRow(ctx,tx,"deal_product",id,live);p.ApplyLocked(ctx,tx,l)}`, true},
		{"another table", `func write(){p.ApplyGuarded(ctx,tx,"offer",id,nil)}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			sites := lineFixture(t, test.source).productPatchSites(map[string]bool{"deal_product": true})
			if (len(sites) > 0) != test.refused {
				t.Fatalf("found %d product patches, expected refusal=%v", len(sites), test.refused)
			}
		})
	}
}

func TestDirectDealPatchesStayInTheirCanonicalSeams(t *testing.T) {
	t.Parallel()
	body := `l,_:=sk.LockRow(ctx,tx,"deal",id,live);if err:=refuseTypedMoneyOnLines(ctx,tx,convert(l.ID()),p);err!=nil{return err};p.ApplyLocked(ctx,tx,l)`
	for _, name := range []string{"applyDealPatchLocked", "copiedGuard"} {
		flow := lineFixture(t, "func "+name+"(){"+body+"}")
		sites := flow.dealPatchSites("deal")
		if len(sites) != 1 {
			t.Fatalf("found %d sites", len(sites))
		}
		if got := flow.canonicalDealPatch(sites[0]); got != (name == "applyDealPatchLocked") {
			t.Fatalf("%s canonical=%v", name, got)
		}
	}
}

func TestLockProvenanceDoesNotFreezeAnUnvisitedCallersAnswer(t *testing.T) {
	t.Parallel()
	flow := lineFixture(t, `func write(w Binding){l,_:=sk.LockRow(ctx,tx,w.Table,id,live);p.ApplyLocked(ctx,tx,l)};func entry(){write(Binding{Table:"offer"})}`)
	for name := range flow.values {
		flow.values[name] = map[string]map[string]bool{}
	}
	for _, name := range []string{"write", "entry", "write"} {
		flow.readFunction(name, flow.functions[name])
	}
	if sites := flow.dealPatchSites("deal"); len(sites) != 0 {
		t.Fatalf("an incomplete first pass invented %d deal writers", len(sites))
	}
	sites := flow.dealPatchSites("offer")
	if len(sites) != 1 || !sites[0].tables["offer"] || sites[0].tables["?"] {
		t.Fatalf("the unrelated writer lost its actual table: %v", sites)
	}
}
