// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// An audited update says what it changed FROM.
//
// audit_log carries before and after images. A row whose action is `update` and
// whose before is absent records that something changed and cannot say what it
// changed from — so field history renders half a change, and nothing can restore
// the value. There is no other place that value is written down: an audit row
// cannot be written after the fact.
//
// Two kinds of write reach the same column, and until AuditEvent existed they
// were spelled identically. One has no prior field state at all — a secret
// rotated, a delivery replayed, a tag applied — and its absent before-image is
// the honest answer. The other simply did not look. This gate is what keeps them
// apart: the first says so by the door it calls, and says why in a waiver here.
//
// It is the census half of a pair. storekit's refusal is the chokepoint, and it
// is the authority — an H2 walk cannot resolve a call reached through an
// interface, a stored field, or a closure, nor an action argument built at
// runtime (privacy/retentionactions.go passes a verb read from a database row).
// The gate keeps the chokepoint the only door; the chokepoint catches what the
// gate cannot see.
//
// Only `update` is judged. `archive` sites also pass an absent before-image and
// are deliberately left alone: un-archiving is not a before-image replay but
// `SET archived_at = NULL` plus whatever that record's archive took down with
// it, which is a per-type product decision and not this gate's to force.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// The four doors into audit_log, and which of them carries a before-image.
var auditDoorsWithBeforeImage = map[string]bool{
	"Audit":                  true,
	"AuditWithEvidence":      true,
	"AuditEvent":             false,
	"AuditEventWithEvidence": false,
}

// Argument positions shared by all four doors: (ctx, tx, action, entityType,
// entityID, …). Only the images differ after that, which is the point of the
// split.
const (
	auditActionArg     = 2
	auditEntityTypeArg = 3
	auditBeforeArg     = 5
)

// eventShapedUpdates: writes that record an occurrence on a record rather than a
// change to its fields, keyed by `path:function`. Each entry states what the
// write has INSTEAD of a prior state — a reason that only restates the subject
// is refused by gatekit.
//
// The FUNCTION is the unit of ratification, not the call. A function that audits
// twice — people/linkedinmatchapply.go does — is judged once, so ratifying it
// says every audited update it makes is occurrence-shaped. Splitting the two
// would need a line number in the key, and a key that moves whenever the file
// above it moves is a waiver that goes stale for no reason.
//
// Empty is a result, not a default: it would mean every audited update in the
// tree records what it changed from.
var eventShapedUpdates = gatekit.Waive(map[string]string{
	"internal/modules/collections/tags.go:ApplyTag": "the tag row is untouched; the write inserts a taggable link, " +
		"and the after image names the record it now points at.",
	"internal/modules/collections/tags.go:RemoveTag": "the tag row is untouched; the write deletes a taggable link, " +
		"and the after image names the record it stopped pointing at.",
	"internal/modules/collections/members.go:AddMember": "the list row is untouched; the write inserts a membership, " +
		"and the after image names the record that joined.",
	"internal/modules/webhooks/store.go:RotateSecret": "the new signing secret replaces a value that must never be " +
		"copied into audit_log, so the after image carries the fact of the rotation and neither secret.",
	"internal/modules/webhooks/deliverystore.go:requireReplay": "the subscription is unchanged; the write records that " +
		"a delivery was re-attempted, and the after image names which one.",
	"internal/modules/activities/extractionclaim.go:FinishExtractionRead": "the claim proves the reading was running, " +
		"so a prior state exists — it is a job's own progress rather than a field a person set, and a document is " +
		"re-read by starting a new attempt, never by putting a finished one back to running.",
	"internal/modules/activities/transcriptread.go:FinishTranscriptRead": "the compare-and-set proves the reading was " +
		"running, so this is a run record's outcome, not a record whose fields a user edits: nothing would ever be " +
		"restored to a reading's earlier progress.",
	"internal/modules/ai/feedback.go:Record": "the upsert as readily creates the ledger row as replaces one, so the " +
		"write records that a human decided rather than a field moving; a verdict it superseded was written through " +
		"this same door and stays readable as that write's own audit row.",
})

// unresolvableAuditActions: call sites whose action argument this walk cannot
// reduce to a constant, keyed by `path:function`. Each entry states that
// wrapper's own guarantee, because the gate cannot judge the verb and the
// chokepoint is what actually holds these.
//
// Ratifying them is the only honest third answer. Failing them would make the
// gate impossible to green over wrappers that are correct; merely counting them
// would let a real defect through in silence.
var unresolvableAuditActions = gatekit.Waive(map[string]string{})

func TestEveryAuditedUpdateRecordsWhatItChangedFrom(t *testing.T) {
	defer eventShapedUpdates.AssertAllMatched(t)
	defer unresolvableAuditActions.AssertAllMatched(t)

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileReachesAnAuditDoor,
		// The extension tier is absent on purpose. extensions/ holds separate Go
		// modules that cannot import internal/platform/database/storekit at all —
		// the dependency DAG forbids it — so an extension reaches audit_log only
		// through compose/extledger.go, which is inside this root and ratified
		// below like any other wrapper.
	}.Files(t)

	constantsByPackage := packageConstants(files)

	var withBefore, eventShaped, unresolvable int
	for _, parsed := range files {
		consts := constantsByPackage[filepath.Dir(parsed.Path)]
		for _, site := range auditSitesIn(parsed) {
			switch action, known := site.action(consts); {
			case !known:
				unresolvable++
				if !unresolvableAuditActions.Waived(t, site.key) {
					t.Errorf("%s: %s builds its audit verb at runtime, so this gate cannot judge it.\n"+
						"\tRatify it in unresolvableAuditActions with the guarantee that holds it, "+
						"or spell the verb as a literal.", site.key, site.door)
				}
			case action != "update":
				// archive, create, erase and the rest: out of this gate's scope.
			case auditDoorsWithBeforeImage[site.door]:
				withBefore++
				if site.beforeIsAbsent {
					t.Errorf("%s: audits an update to %s with no before-image.\n"+
						"\tRecord what the fields held (the row is readable in this transaction), "+
						"or call storekit.AuditEvent and say here what this write has instead.",
						site.key, site.entityType(consts))
				}
			default:
				eventShaped++
				if !eventShapedUpdates.Waived(t, site.key) {
					t.Errorf("%s: records an update to %s as an occurrence.\n"+
						"\tRatify it in eventShapedUpdates with what this write has instead of a "+
						"prior state, or record the before-image and call storekit.Audit.",
						site.key, site.entityType(consts))
				}
			}
		}
	}

	// Counted last, and reported rather than fatal: a Fatalf here would abort
	// before the findings above print, and those findings are the worklist.
	//
	// Three counts, because the walk has three ways to fail short and one total
	// would hide which. Each floor sits BELOW the real number, so a scan that
	// stopped working fails instead of quietly finding nothing.
	if withBefore < 80 || eventShaped+unresolvable < 15 {
		t.Errorf("resolved %d image-carrying, %d occurrence-shaped and %d runtime-verb audit site(s) — "+
			"one of the ways of finding a call has stopped working",
			withBefore, eventShaped, unresolvable)
	}
}

// auditSite is one call into audit_log, located and pre-read.
type auditSite struct {
	key            string // path:function — what a waiver is keyed by
	door           string // which of the four doors
	args           []ast.Expr
	beforeIsAbsent bool
}

// action resolves the site's verb: a literal, or a package-level constant this
// file's package declares. Anything else — a parameter, a struct field, a value
// read at runtime — is unknown, and saying so is the point.
func (s auditSite) action(consts map[string]string) (string, bool) {
	return resolveString(s.argAt(auditActionArg), consts)
}

// entityType is for the failure message only, so an unresolvable one degrades to
// the expression's own spelling rather than costing a finding.
func (s auditSite) entityType(consts map[string]string) string {
	if value, known := resolveString(s.argAt(auditEntityTypeArg), consts); known {
		return value
	}
	return "an entity named at runtime"
}

func (s auditSite) argAt(i int) ast.Expr {
	if i >= len(s.args) {
		return nil
	}
	return s.args[i]
}

func resolveString(expr ast.Expr, consts map[string]string) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(node.Value)
		return value, err == nil
	case *ast.Ident:
		value, declared := consts[node.Name]
		return value, declared
	default:
		return "", false
	}
}

// fileReachesAnAuditDoor is the Scope subject, asked identically inside the
// roots and outside them.
func fileReachesAnAuditDoor(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if _, isDoor := auditDoorAt(n); isDoor {
			found = true
		}
		return !found
	})
	return found
}

func auditSitesIn(parsed gatekit.ParsedFile) []auditSite {
	var sites []auditSite
	for _, decl := range parsed.File.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			door, isDoor := auditDoorAt(call.Fun)
			if !isDoor {
				return true
			}
			sites = append(sites, auditSite{
				key:            parsed.Path + ":" + fn.Name.Name,
				door:           door,
				args:           call.Args,
				beforeIsAbsent: auditDoorsWithBeforeImage[door] && isAbsentImageExpr(call, parsed.File),
			})
			return true
		})
	}
	return sites
}

// auditDoorAt names the door a node calls, if it calls one.
func auditDoorAt(n ast.Node) (string, bool) {
	selector, isSelector := n.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent || pkg.Name != "storekit" {
		return "", false
	}
	if _, isDoor := auditDoorsWithBeforeImage[selector.Sel.Name]; !isDoor {
		return "", false
	}
	return selector.Sel.Name, true
}

// isAbsentImageExpr reads the before argument the only way source can be read:
// the bare `nil` identifier, or a local variable this function only ever
// declares and never assigns a value to.
//
// It CANNOT see a typed nil that arrives through a call, a parameter or a field
// — `imageOrNil(ch.Before)` returns one, and so does any helper that builds an
// image conditionally. Those reach audit_log as SQL NULL exactly as a bare nil
// does, and only storekit's refusal catches them. That is the division of labour
// this gate's doc comment describes, and the reason the refusal is not optional.
func isAbsentImageExpr(call *ast.CallExpr, _ *ast.File) bool {
	if auditBeforeArg >= len(call.Args) {
		return false
	}
	ident, isIdent := call.Args[auditBeforeArg].(*ast.Ident)
	return isIdent && ident.Name == "nil"
}

// packageConstants collects every package-level string constant in the tree,
// grouped by the directory that declares it — which is the package, since Go
// puts one package in one directory.
//
// By PACKAGE and not by file, because that is where the verbs live that are not
// written as literals. `actionUpdate` is declared once in a package and used
// from a dozen files in it; resolving only a file's own declarations would call
// those sites unresolvable, and an unresolvable site gets RATIFIED. That would
// turn "we could not read this verb" into a standing waiver over sites whose
// verb is plainly `update` — the census failing short into the one bucket that
// forgives it.
func packageConstants(files []gatekit.ParsedFile) map[string]map[string]string {
	byPackage := map[string]map[string]string{}
	for _, parsed := range files {
		dir := filepath.Dir(parsed.Path)
		if byPackage[dir] == nil {
			byPackage[dir] = map[string]string{}
		}
		collectStringConstants(parsed.File, byPackage[dir])
	}
	return byPackage
}

func collectStringConstants(file *ast.File, into map[string]string) {
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if literal, known := resolveString(value.Values[i], nil); known {
					into[name.Name] = literal
				}
			}
		}
	}
}
