// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Which record types a scrub verb is ever written against.
//
// scrubverbs_test.go asks the neighbouring question — which verbs certify a
// scrub — and derives its corpus from privacy's writers so a new one cannot
// arrive unjudged. This asks what those verbs are written ABOUT, because a
// reader outside privacy has to decide whether the boundary is live for the
// records it reads.
//
// The readers stop at the newest tombstone of the record they read. In
// privacy they are field, record and edge history and the compliance log.
// Through UnscrubbedImageSQL they are compose's export, undo, restore, task
// history, automatic work, magic and weekly reads.
//
// The weekly review is such a reader. Its deal block reconstructs a deal's state
// as of the week's closing instant by reverse-applying audit images, and it
// stops at a scrub tombstone: the spine is append-only, so the images a scrub
// certified gone are still physically there, and folding one back would put
// erased data onto a rep's scorecard. Today no writer in the tree tombstones a
// `deal`, so that boundary excludes nothing and the reader's shortfall count is
// always zero.
//
// THAT IS THE FACT THIS GATE PINS, and the reason it is a gate rather than a
// comment. "No deal is ever scrubbed" is true today and invisible if it stops
// being true: deal erasure would land, the weekly's boundary would quietly start
// firing, and nobody would have decided that a rep's frozen week may now report
// a shortfall. The census fails on that day and asks for the decision.
//
// A record type ADDED to the scrub set breaks this and asks for the decision.
// One listed that the walk no longer finds breaks it too. Either no writer
// scrubs it now, or the walk stopped reading the writer that does.
//
// The corpus is the audit census, not one module. The walk starts at the
// INSERT storekit holds and climbs each writer's callers, carrying the verb and
// the record type together until both are constants.

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// scrubbedEntityTypes is what the tree tombstones today, and the list a reader
// of the erasure boundary may rely on.
//
// `deal` is deliberately ABSENT and that absence is load-bearing — see the file
// comment. Adding it here is a product decision about frozen weekly reviews, not
// a test fix.
var scrubbedEntityTypes = map[string]bool{
	// A review has no restore endpoint, and weekly deal reconstruction reads none
	// of its audit images.
	"legal_review": true,
	// These records have no restore endpoint. Their audit readers stop at erasure;
	// weekly deal reconstruction reads none of their images.
	"data_subject_request":   true,
	"deal_room_comment":      true,
	"deal_room_session":      true,
	"deal_room_thread":       true,
	"dedupe_candidate":       true,
	"intro_request":          true,
	"weekly_plan_commitment": true,
	// Approval verdicts remain in their row and audit verb. Erased proposal images
	// end at this boundary; deal reconstruction never reads approval images.
	"approval":              true,
	"contact":               true,
	"lead":                  true,
	"activity":              true,
	"attachment":            true,
	"deal_room_participant": true,
	"scheduled_send":        true,
	// The review over a message the erasure just cancelled. It rides in beside
	// scheduled_send because it is the same act: the held message is emptied
	// and its review is closed in one transaction, so a reader that stops at
	// one stops at the other.
	//
	// THE NAMED READER IS UNAFFECTED. The weekly review's deal block
	// reconstructs a DEAL from its own audit images (compose/weekly), and a
	// review is not a deal and never appears in that reconstruction — so a
	// review tombstone cannot turn a frozen week into a shortfall it never
	// reported.
	//
	// The other readers of the boundary (compose/undoabilitypage.go,
	// recordrestoreseam.go, magic/done.go, export.go) are each parameterised by
	// the entity the caller named. Only somebody asking about that review sees
	// this tombstone. No surface offers undo or restore on
	// one. The review endpoints read the row directly and never reverse-apply
	// an image.
	"communication_review":  true,
	"voice_learning_signal": true,
	"ai_call_payload":       true,
	// A stage-evidence row whose quotation of an erased message was cleared.
	// The weekly review rebuilds a DEAL from its own audit images, never an
	// evidence row. The evidence create images never carried the quotation.
	"deal_stage_evidence": true,
	// Notice history reads stored text directly; no restore surface may replay its erased images.
	"notice": true,
	// A signal about the erased contact, tombstoned so its audit images, which
	// keep a corrected kind and evidence, are withheld. The weekly review
	// rebuilds a DEAL from its own audit images and never reads a signal's.
	"signal": true,
	// An extension's own table. Core has no restore or undo for a unit's row.
	// The weekly review reads deals. The compliance log is the one reader, and it
	// hides these images.
	extensionRecordTypes: true,
	// Retention erases these rows outright and certifies it, so their earlier
	// images are redacted. The weekly review rebuilds a DEAL from its own audit
	// images. A risk day, a stored original and a report edition are not deals.
	"deal_risk_day":  true,
	"raw_capture":    true,
	"report_edition": true,
	// Removing a configured entry, a rate or a voice sample writes `erase` with
	// the removed row as its before-image, so field history ends there. None is
	// a deal, and the weekly review reads none of their images.
	"ai_model_rate":          true,
	"lead_disqualify_reason": true,
	"lead_origin":            true,
	"lead_source":            true,
	"partner_stage":          true,
	"voice_corpus_source":    true,
	"voice_profile":          true,
}

// runtimeScrubVerbs names, for a writer whose verb the walk cannot read, the
// record types it can scrub, drawn from the Go that decides the verb.
var runtimeScrubVerbs = map[string]func(t *testing.T, scrub map[string]bool) []string{
	// The verb is a column of the policy row. Each object it may act on is
	// paired with its actions in retentionActions.
	"backend/internal/modules/privacy/retentionactions.go:auditRetentionAction": retentionScrubbedObjects,
	// Each setting declares its verb, and its change is audited under its object.
	"backend/internal/platform/config/spec.go:DefineStored":  settingScrubbedObjects,
	"backend/internal/platform/config/sealed.go:sealedEntry": settingScrubbedObjects,
	// The verb is the unit's own, and the record is a row of its own table.
	"backend/internal/compose/extledger.go:recordExtensionChange": func(*testing.T, map[string]bool) []string { return extensionTypes(nil) },
	// Trail.Resolve admits update and restore, and neither is a scrub.
	"backend/internal/platform/database/storekit/auditverb.go:AuditWithTrail": scrubsNothing,
	// The verb is a commission decision word, or what auditVerb returns for one.
	"backend/internal/modules/commissions/decide.go:decideTx": commissionScrubs,
	"backend/internal/modules/commissions/decide.go:voidOne":  commissionScrubs,
}

func scrubsNothing(*testing.T, map[string]bool) []string { return nil }

// commissionScrubs reads the commission verbs: each Decision constant and each
// constant auditVerb returns. None may be a scrub, because no record type here
// says which reader would then stop at it.
func commissionScrubs(t *testing.T, scrub map[string]bool) []string {
	t.Helper()
	pkg := typedPackageOf(t, modulePath+"/internal/modules/commissions")
	var verbs []string
	for _, name := range pkg.types.Scope().Names() {
		if constant, ok := pkg.types.Scope().Lookup(name).(*types.Const); ok && strings.HasPrefix(name, "Decision") {
			verbs = append(verbs, typedConstString(constant))
		}
	}
	verbs = append(verbs, returnedConstants(pkg, "auditVerb")...)
	if len(verbs) < 2 {
		t.Fatalf("read %v as the commission verbs, too few to be the decisions", verbs)
	}
	for _, verb := range verbs {
		if scrub[verb] {
			t.Errorf("the commission verb %q is a scrub: name the record type it scrubs in runtimeScrubVerbs", verb)
		}
	}
	return nil
}

func typedPackageOf(t *testing.T, path string) *typedPackage {
	t.Helper()
	pkgs := typedModule(t)
	for i := range pkgs {
		if pkgs[i].path == path {
			return &pkgs[i]
		}
	}
	t.Fatalf("the module has no package %s", path)
	return nil
}

func typedConstString(c *types.Const) string {
	if c.Val().Kind() != constant.String {
		return ""
	}
	return constant.StringVal(c.Val())
}

// returnedConstants are the string constants function name returns directly.
func returnedConstants(pkg *typedPackage, name string) []string {
	var values []string
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != name || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if ret, isReturn := n.(*ast.ReturnStmt); isReturn && len(ret.Results) == 1 {
					if value, isConst := auditConstString(pkg.info, ret.Results[0]); isConst {
						values = append(values, value)
					}
				}
				return true
			})
		}
	}
	return values
}

func settingScrubbedObjects(_ *testing.T, scrub map[string]bool) []string {
	var objects []string
	for _, spec := range compose.SettingsCatalogForTest() {
		if scrub[spec.AuditVerb] {
			objects = append(objects, spec.Object)
		}
	}
	return objects
}

// retentionScrubbedObjects reads the objects of retentionActions whose action
// is a scrub, and the extension tables when extensionActions holds a scrub.
func retentionScrubbedObjects(t *testing.T, scrub map[string]bool) []string {
	t.Helper()
	var objects []string
	if slices.ContainsFunc(privacyExtensionActions(t), func(action string) bool { return scrub[action] }) {
		objects = append(objects, extensionRecordTypes)
	}
	for _, pair := range auditPackageMapKeys(t, modulePath+"/internal/modules/privacy", "retentionActions") {
		object, action, _ := strings.Cut(pair, "/")
		if scrub[action] {
			objects = append(objects, object)
		}
	}
	return objects
}

// privacyExtensionActions reads what a retention policy may do to an
// extension's row, from privacy's extensionActions.
func privacyExtensionActions(t *testing.T) []string {
	t.Helper()
	pkg := typedPackageOf(t, modulePath+"/internal/modules/privacy")
	list, ok := pkg.types.Scope().Lookup("extensionActions").(*types.Var)
	if !ok {
		t.Fatal("privacy declares no extensionActions, the list this gate reads for an extension's row")
	}
	var actions []string
	for _, file := range pkg.files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || pkg.info.Defs[spec.Names[0]] != list || len(spec.Values) != 1 {
				return true
			}
			if lit, isLit := spec.Values[0].(*ast.CompositeLit); isLit {
				for _, elt := range lit.Elts {
					if value, isConst := auditConstString(pkg.info, elt); isConst {
						actions = append(actions, value)
					}
				}
			}
			return false
		})
	}
	if len(actions) == 0 {
		t.Fatal("privacy's extensionActions is no list of constants this gate can read")
	}
	return actions
}

func TestEveryScrubbedRecordTypeIsOneAReaderOfTheBoundaryExpects(t *testing.T) {
	t.Parallel()
	walk := newTombstoneWalk(t, auditEntityCensusOf(t), scrubVerbSet(t, privacyPackageConstants(t)))
	for _, finding := range walk.unread {
		t.Error(finding)
	}
	for key := range runtimeScrubVerbs {
		if !walk.ratified[key] {
			t.Errorf("runtimeScrubVerbs names %s, whose audit verb this walk now reads or no longer meets", key)
		}
	}
	if len(walk.found) == 0 {
		t.Fatal("the walk read no scrub tombstone, so it reads a smaller tree than it thinks, " +
			"which is the one way it must not fail")
	}
	for _, entity := range sortedKeys(walk.found) {
		if scrubbedEntityTypes[entity] {
			continue
		}
		t.Errorf("%s writes a scrub tombstone against %q, which this census does "+
			"not list.\n\nEvery reader that reverse-applies audit images for %[2]q now "+
			"stops at that tombstone. Decide what each should do, then add %[2]q here.\n"+
			"The weekly review's deal block is one such reader "+
			"(compose/weekly/weeklyweekend.go): a %[2]q scrub makes a rep's frozen week "+
			"report a shortfall it never reported before.", walk.found[entity], entity)
	}
	for _, entity := range sortedKeys(scrubbedEntityTypes) {
		if _, found := walk.found[entity]; !found {
			t.Errorf("scrubbedEntityTypes lists %q, and the walk finds no scrub tombstone against it. "+
				"Remove it if no writer scrubs it now, or find the writer the walk stopped reading.", entity)
		}
	}
}

func TestTheTombstoneWalkCarriesTheVerbAndTypeUpTheirCallers(t *testing.T) {
	t.Parallel()
	census := fixtureCensus(t, fixtureSeed, map[string]string{"a.go": fixtureHeader +
		"type eraser interface{ Erase(t string) error }\ntype store struct{}\n" +
		"func (store) Erase(t string) error { return k.Audit(ctx, tx, \"erase\", t) }\n" +
		"func viaInterface(e eraser) { _ = e.Erase(\"alpha\") }\n" +
		"func write(verb, t string) error { return k.Audit(ctx, tx, verb, t) }\n" +
		"func callers() { _ = write(\"erase\", \"beta\"); _ = write(\"update\", \"never\") }\n" +
		"func eraseX(verb string) error { return k.Audit(ctx, tx, verb, \"delta\") }\n" +
		"func erasesDelta() { _ = eraseX(\"erase\") }\n" +
		"func fromData(verbs []string) { _ = k.Audit(ctx, tx, verbs[0], \"gamma\") }\n"})
	walk := newTombstoneWalk(t, census, map[string]bool{"erase": true})
	if got := sortedKeys(walk.found); !slices.Equal(got, []string{"alpha", "beta", "delta"}) {
		t.Errorf("the walk found scrubs against %v, want alpha through the interface, beta through the wrapper and delta through the verb alone", got)
	}
	// The planted storekit's FromData reads its verb from data too.
	if len(walk.unread) != 2 || !slices.ContainsFunc(walk.unread, func(finding string) bool { return strings.Contains(finding, "verbs[0]") }) {
		t.Errorf("the walk reported %v, want the two verbs read from data", walk.unread)
	}
}

// extensionRecordTypes stands for the record types an extension declares
// (`ext_<unit>_<table>`). Their names are data, not constants.
const extensionRecordTypes = "ext_*"

// tombstoneSlot holds a verb or a record type while the walk climbs. It is
// constant values, or a parameter of the function the walk is in.
type tombstoneSlot struct {
	values []string
	param  int
}

func slotParam(at int) tombstoneSlot { return tombstoneSlot{param: at} }

func slotValues(values []string) tombstoneSlot { return tombstoneSlot{values: values, param: -1} }

func (s tombstoneSlot) String() string {
	if s.param >= 0 {
		return fmt.Sprint(s.param)
	}
	return strings.Join(s.values, ",")
}

// tombstoneWalk climbs from storekit's INSERT to every call that fixes both
// the verb and the record type of an audit row.
type tombstoneWalk struct {
	t     *testing.T
	index *auditIndex
	scrub map[string]bool
	// found maps each scrubbed record type to one place that writes it.
	found map[string]string
	// unread are the writes whose verb or record type the walk cannot read.
	unread []string
	// ratified are the writers a vocabulary answered for.
	ratified map[string]bool
	seen     map[string]bool
}

func newTombstoneWalk(t *testing.T, census *auditEntityCensus, scrub map[string]bool) *tombstoneWalk {
	t.Helper()
	w := &tombstoneWalk{
		t: t, index: census.index, scrub: scrub,
		found: map[string]string{}, ratified: map[string]bool{}, seen: map[string]bool{},
	}
	decl := census.index.decls[census.seed]
	if decl == nil {
		t.Fatalf("%s is not declared, so the walk starts from nothing", census.seed)
	}
	info := census.index.declPkg[decl].info
	verb, entity := auditParamIndex(info, decl, "action"), auditParamIndex(info, decl, auditSeedParam)
	if verb < 0 || entity < 0 {
		t.Fatalf("%s no longer takes an action and an %s, the two this walk carries", census.seed, auditSeedParam)
	}
	w.climb(census.seed, slotParam(verb), slotParam(entity))
	return w
}

// climb follows a function's audit write to every call of it, and to every
// call through an interface method it implements.
func (w *tombstoneWalk) climb(key string, verb, entity tombstoneSlot) {
	id := key + "|" + verb.String() + "|" + entity.String()
	if w.seen[id] {
		return
	}
	w.seen[id] = true
	reached, _ := w.index.interfacesFor(key)
	for _, k := range append([]string{key}, reached...) {
		for _, use := range w.index.uses[k] {
			// A writer named but not called is reported by the census itself.
			if use.call != nil {
				w.atCall(use, verb, entity)
			}
		}
	}
}

func (w *tombstoneWalk) atCall(use auditFuncUse, verb, entity tombstoneSlot) {
	writer := auditWriterName(use)
	verbs, ok := w.read(use, verb)
	if !ok {
		w.unreadVerb(use, writer, verb)
		return
	}
	scrubs := slices.DeleteFunc(slices.Clone(verbs.values), func(v string) bool { return !w.scrub[v] })
	if len(scrubs) == 0 && len(verbs.params) == 0 {
		return
	}
	entities, ok := w.read(use, entity)
	if !ok {
		w.unreadEntity(use, writer, entity)
		return
	}
	if len(scrubs) > 0 {
		for _, value := range entities.values {
			if _, known := w.found[value]; !known {
				w.found[value] = auditRepoPos(use.pos)
			}
		}
		for _, forwarded := range entities.params {
			w.climb(forwarded.key, slotValues(scrubs), slotParam(forwarded.param))
		}
	}
	for _, forwarded := range verbs.params {
		if len(entities.values) > 0 {
			w.climb(forwarded.key, slotParam(forwarded.param), slotValues(entities.values))
		}
		for _, alongside := range entities.params {
			if alongside.key == forwarded.key {
				w.climb(forwarded.key, slotParam(forwarded.param), slotParam(alongside.param))
			}
		}
	}
}

// read is what a slot holds at one call into the function that has it.
func (w *tombstoneWalk) read(use auditFuncUse, slot tombstoneSlot) (auditResolved, bool) {
	if slot.param < 0 {
		return auditResolved{values: slot.values}, true
	}
	at := slot.param + use.shift
	if at >= len(use.call.Args) || w.variadicAt(use, at) {
		return auditResolved{}, false
	}
	return w.index.resolver(use).resolve(use.call.Args[at])
}

// variadicAt reports whether argument at fills a variadic parameter, whose
// position does not name one argument.
func (w *tombstoneWalk) variadicAt(use auditFuncUse, at int) bool {
	sig, ok := types.Unalias(use.pkg.info.Types[use.call.Fun].Type).(*types.Signature)
	return !ok || (sig.Variadic() && at >= sig.Params().Len()-1)
}

func (w *tombstoneWalk) unreadVerb(use auditFuncUse, writer string, verb tombstoneSlot) {
	if vocabulary, known := runtimeScrubVerbs[writer]; known {
		w.ratified[writer] = true
		w.foundAll(use, vocabulary(w.t, w.scrub))
		return
	}
	w.unread = append(w.unread, fmt.Sprintf("%s writes an audit row whose verb is %s, which this walk cannot read, "+
		"so whether it scrubs is judged by nobody.\n\tPass the verb as a constant, or name the record types it can "+
		"scrub in runtimeScrubVerbs, keyed %q.", auditRepoPos(use.pos), w.argText(use, verb), writer))
}

func (w *tombstoneWalk) unreadEntity(use auditFuncUse, writer string, entity tombstoneSlot) {
	if vocabulary, known := runtimeTypedAuditWriters[writer]; known {
		w.foundAll(use, vocabulary(w.t))
		return
	}
	w.unread = append(w.unread, fmt.Sprintf("%s writes a scrub tombstone whose record type is %s, which this walk "+
		"cannot read.\n\tPass the record type as a constant, or name its Go vocabulary in "+
		"runtimeTypedAuditWriters, keyed %q.", auditRepoPos(use.pos), w.argText(use, entity), writer))
}

func (w *tombstoneWalk) foundAll(use auditFuncUse, entities []string) {
	for _, value := range entities {
		if _, known := w.found[value]; !known {
			w.found[value] = auditRepoPos(use.pos)
		}
	}
}

func (w *tombstoneWalk) argText(use auditFuncUse, slot tombstoneSlot) string {
	if at := slot.param + use.shift; slot.param >= 0 && at < len(use.call.Args) {
		return types.ExprString(use.call.Args[at])
	}
	return types.ExprString(use.call)
}
