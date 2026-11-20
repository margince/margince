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
// It fails in the useful direction. A record type ADDED to the scrub set breaks
// this; one removed does not, because a shrinking set cannot surprise a reader
// that already handles the boundary.

import (
	"go/ast"
	"go/token"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/corereach"
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
	"ai_call":               true,
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
	// The retention pass erases these on their policy. Each is a record of its
	// own, and the weekly review rebuilds a DEAL from deal images alone.
	"deal_risk_day":  true,
	"raw_capture":    true,
	"report_edition": true,
	// An extension's own table. Core has no restore or undo for a unit's row.
	// The weekly review reads deals. The compliance log is the one reader, and it
	// hides these images.
	extensionScrubbedTypes: true,
}

func TestEveryScrubbedRecordTypeIsOneAReaderOfTheBoundaryExpects(t *testing.T) {
	t.Parallel()
	found, opaque, unreadVerbs := repoAuditCensus(t).scrubbedTypes(t)
	for _, site := range opaque {
		t.Errorf("%s stamps a scrub tombstone whose record type is not a literal, so "+
			"this census cannot read it and would pass without ever seeing it.\n"+
			"Spell the entity type at the call site, or extend scrubbedTypes "+
			"to resolve it.", site)
	}
	for _, site := range unreadVerbs {
		t.Errorf("%s audits a privacy write whose verb this census cannot read, so it "+
			"cannot tell whether the row is a scrub tombstone.\n"+
			"Pass the verb as privacy's constant or a literal.", site)
	}
	if len(found) == 0 {
		t.Fatal("no scrub tombstone writer found in the privacy module — this census " +
			"reads a smaller tree than it thinks, which is the one way it must not fail")
	}

	var unexpected []string
	for entity := range found {
		if !scrubbedEntityTypes[entity] {
			unexpected = append(unexpected, entity)
		}
	}
	sort.Strings(unexpected)
	for _, entity := range unexpected {
		t.Errorf("privacy writes a scrub tombstone against %q, which this census does "+
			"not list.\n\nEvery reader that reverse-applies audit images for %[1]q now "+
			"stops at that tombstone. Decide what each should do, then add %[1]q here.\n"+
			"The weekly review's deal block is one such reader "+
			"(compose/weekly/weeklyweekend.go): a %[1]q scrub makes a rep's frozen week "+
			"report a shortfall it never reported before.", entity)
	}
}

// scrubbedTypes reads the record type of every scrub tombstone the
// privacy module writes, from the audit write census (auditwrites_test.go). A
// privacy write whose verb it cannot read is returned apart.
//
// Three sites take their type at runtime from a declaration. redactDeclaredTables
// stamps the declared tables' types, an extension row's table is the ext_*
// class, and the retention pass reads its own dispatch table.
func (c *auditCensus) scrubbedTypes(t *testing.T) (found map[string]bool, opaque, unreadVerbs []string) {
	t.Helper()
	wrappers := c.privacyScrubWrappers()
	found = map[string]bool{}
	for _, site := range c.sites {
		viaWrapper := c.callsPrivacyWrapper(site, wrappers)
		if site.asValue || (site.src.dir != privacyDir && !viaWrapper) {
			continue
		}
		if site.fn == "apply" && strings.HasSuffix(site.src.path, "/retentionactions.go") {
			for _, object := range c.retentionScrubObjects(t) {
				found[object] = true
			}
			continue
		}
		scrub, readable := viaWrapper, true
		if !viaWrapper {
			scrub, readable = c.actionScrubs(site.at, site.writer, 0)
		}
		switch {
		case !scrub:
		case c.isExtensionEntity(site):
			// A row of an extension's own table is that class whatever its verb.
			found[extensionScrubbedTypes] = true
		case !readable:
			unreadVerbs = append(unreadVerbs, site.pos)
		case site.opaque == "":
			for _, entity := range site.types {
				found[entity] = true
			}
		case site.fn == "redactDeclaredTables":
			for _, o := range corereach.Tables() {
				if len(o.Table.Erase) > 0 {
					found[o.Table.Name] = true
				}
			}
		default:
			opaque = append(opaque, site.pos)
		}
	}
	return found, opaque, unreadVerbs
}

// retentionScrubObjects are the objects the retention pass scrubs, by the
// verb each pair in its dispatch table names.
func (c *auditCensus) retentionScrubObjects(t *testing.T) []string {
	t.Helper()
	var objects []string
	for _, pair := range c.packageMapKeys(t, privacyDir, "retentionActions") {
		object, verb, _ := strings.Cut(pair, "/")
		if isScrubVerb(verb) {
			objects = append(objects, object)
		}
	}
	return objects
}

func isScrubVerb(verb string) bool {
	return verb == "erase" || verb == "anonymize" || verb == "restrict"
}

// actionScrubs reads the verb a writer call audits with, from the writer's
// action argument. A writer that chooses its verb inside is read through the
// calls it hands its record type on to. A verb it cannot read reports
// unreadable, and counts as a scrub.
func (c *auditCensus) actionScrubs(at auditCall, writer auditFuncKey, depth int) (scrub, readable bool) {
	decl := c.funcs[writer]
	if decl == nil || depth > auditResolveDepth {
		return true, false
	}
	_, shift := c.match(at.src, at.fn, unindexed(at.call.Fun), writer)
	if i := auditParamIndex(decl, "action"); i >= 0 {
		if i+shift >= len(at.call.Args) {
			return true, false
		}
		values, ok := c.resolve(at.src, at.fn, at.closures, at.call.Args[i+shift], 0)
		return !ok || slices.ContainsFunc(values, isScrubVerb), ok
	}
	readable, seen := true, false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		inner := auditCall{src: c.srcOf[decl], fn: decl, call: call}
		for key, indexes := range c.writers {
			if key.name != auditCalleeName(call) {
				continue
			}
			matched, innerShift := c.match(inner.src, decl, unindexed(call.Fun), key)
			for index := range indexes {
				if !matched || index+innerShift >= len(call.Args) || !c.handsOn(decl, writer, call.Args[index+innerShift]) {
					continue
				}
				seen = true
				innerScrub, innerReadable := c.actionScrubs(inner, key, depth+1)
				scrub, readable = scrub || innerScrub, readable && innerReadable
			}
		}
		return true
	})
	return scrub || !seen, readable && seen
}

// handsOn reports whether arg is one of the parameters writer receives a
// record type at.
func (c *auditCensus) handsOn(decl *ast.FuncDecl, writer auditFuncKey, arg ast.Expr) bool {
	ident, ok := unconverted(arg).(*ast.Ident)
	if !ok {
		return false
	}
	for index := range c.writers[writer] {
		if auditParamIndex(decl, ident.Name) == index {
			return true
		}
	}
	return false
}

// callsPrivacyWrapper reports whether a site, in any package, calls one of
// privacy's scrub wrappers.
func (c *auditCensus) callsPrivacyWrapper(site auditWriteSite, wrappers map[string]bool) bool {
	name := auditCalleeName(site.call)
	if !wrappers[name] {
		return false
	}
	candidates := append([]auditFuncKey{{dir: privacyDir, name: name}}, c.methods[privacyDir+"."+name]...)
	for _, key := range candidates {
		if matched, _ := c.match(site.src, site.at.fn, unindexed(site.call.Fun), key); matched {
			return true
		}
	}
	return false
}

// extensionScrubbedTypes stands for the record types an extension declares
// (`ext_<unit>_<table>`). Their names are data, not literals. The one site that
// scrubs them names the entity as the Table of a cleared row, and this walk
// reads that shape as this class.
const extensionScrubbedTypes = "ext_*"

// extensionClearPath and extensionClearWriter name the scrub whose record type
// is an extprivacy.Cleared row's Table, which holds a table a unit declared.
const (
	extensionClearPath   = "backend/internal/modules/privacy/erasure_extension.go"
	extensionClearWriter = "clearExtensionRows"
)

// isExtensionEntity reports whether a site's record type is an extension
// table's name. Every value it resolves to is one, or it is a field of a value
// an extension package types. The named site's cleared row Table is one too.
func (c *auditCensus) isExtensionEntity(site auditWriteSite) bool {
	if len(site.types) > 0 {
		return !slices.ContainsFunc(site.types, func(entity string) bool { return !strings.HasPrefix(entity, "ext_") })
	}
	sel, ok := site.entity.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if site.src.path == extensionClearPath && funcName(site.at.fn) == extensionClearWriter && sel.Sel.Name == "Table" {
		return true
	}
	typeDir, _, typed := c.exprType(site.src, site.at.fn, sel.X, 0)
	return typed && strings.HasPrefix(typeDir, "backend/pkg/extension/")
}

// scrubVerbConstants are privacy's own names for the three verbs that certify a
// scrub. scrubverbs_test.go holds the list itself against the module's writers;
// this names the identifiers so a call passing one is recognised.
var scrubVerbConstants = map[string]bool{
	"actionErase": true, "actionAnonymize": true, "actionRestrict": true,
}

// carriesScrubVerb reports whether a call is handed one of the three scrub verbs,
// by privacy's constant or as a literal. A prefix match on "action" would also
// admit actionArchive, which certifies no scrub.
func carriesScrubVerb(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		switch a := arg.(type) {
		case *ast.Ident:
			if scrubVerbConstants[a.Name] {
				return true
			}
		case *ast.BasicLit:
			value, err := strconv.Unquote(a.Value)
			if a.Kind == token.STRING && err == nil && (value == "erase" || value == "anonymize" || value == "restrict") {
				return true
			}
		}
	}
	return false
}

// privacyDir is the privacy module, whose audit writes this census reads.
const privacyDir = "backend/internal/modules/privacy"

// privacyScrubWrappers are the privacy functions that carry the verb inside
// themselves, directly or through another such function. Each call to one
// stamps a tombstone, and its arguments name only the record type.
func (c *auditCensus) privacyScrubWrappers() map[string]bool {
	var funcs []*ast.FuncDecl
	for _, src := range c.sources {
		for _, decl := range src.file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && src.dir == privacyDir {
				funcs = append(funcs, fn)
			}
		}
	}
	wrappers := map[string]bool{}
	for grew := true; grew; {
		grew = false
		for _, fn := range funcs {
			if wrappers[fn.Name.Name] {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if isCall && (carriesScrubVerb(call) || wrappers[calleeName(call)]) {
					wrappers[fn.Name.Name], grew = true, true
				}
				return !wrappers[fn.Name.Name]
			})
		}
	}
	return wrappers
}
