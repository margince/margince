// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The audit write census, read at storekit's doors. A door is an exported
// storekit writer whose verb its caller chooses. A site is every call into one
// from outside storekit. A wrapper that hands its parameters on is a writer to
// the census, and its own call to a door is still a site here. The verb and
// the images are judged where they enter storekit.

import (
	"go/ast"
	"go/parser"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// auditTrailType is the verb type a door may take instead of an action
// string. Trail.Resolve admits update and restore and nothing else, so every
// call through such a door is update-shaped without the verb being read.
const auditTrailType = "AuditTrail"

// auditDoor is one storekit door and where its arguments sit. A position is -1
// when the door has no such parameter.
type auditDoor struct {
	name                        string
	verb, entity, before, after int
	verbIsTrail                 bool
}

// auditSite is one call into a door, outside storekit.
type auditSite struct {
	census *auditCensus
	src    *auditSource
	// fn is the function the call sits in, so a reader can follow an image the
	// call names rather than spells.
	fn   *ast.FuncDecl
	door auditDoor
	call *ast.CallExpr
}

// key is path:function, the path relative to the backend module: what a
// waiver is keyed by.
func (s auditSite) key() string {
	return strings.TrimPrefix(s.src.path, "backend/") + ":" + funcName(s.fn)
}

func (s auditSite) argAt(i int) ast.Expr {
	if i < 0 || i >= len(s.call.Args) {
		return nil
	}
	return s.call.Args[i]
}

// verbs are the values the verb can hold: literals, constants, and what a
// called function returns. The enclosing function's parameters and locals are
// not followed. A wrapper handed its verb stays unknown, and is ratified with
// the guarantee its callers keep.
func (s auditSite) verbs() ([]string, bool) {
	if s.door.verbIsTrail {
		return []string{"update"}, true
	}
	return s.strings(s.argAt(s.door.verb))
}

// entityType is for a failure message only, so an unresolvable one degrades to
// a description rather than costing a finding.
func (s auditSite) entityType() string {
	if values, known := s.strings(s.argAt(s.door.entity)); known {
		return strings.Join(values, " or ")
	}
	return "an entity named at runtime"
}

func (s auditSite) strings(expr ast.Expr) ([]string, bool) {
	if expr == nil {
		return nil, false
	}
	return s.census.resolve(s.src, nil, nil, expr, 0)
}

// beforeIsAbsent reads the before argument the one way source can be read
// here: the bare `nil`, written at the call. A variable assigned nil, a
// parameter, or a helper that builds an image conditionally also reaches
// audit_log as SQL NULL. Only storekit's refusal catches those.
func (s auditSite) beforeIsAbsent() bool {
	ident, isIdent := s.argAt(s.door.before).(*ast.Ident)
	return isIdent && ident.Name == "nil"
}

// auditDoorSites answers every call into a door outside storekit, and every
// place a door is named other than as the function a call makes.
func auditDoorSites(t *testing.T) (sites []auditSite, asValues []string) {
	t.Helper()
	auditWriteSites(t)
	sites, asValues = auditCensusRead.doorSites()
	if len(sites) == 0 {
		t.Fatal("the audit census found no call into a storekit door, so every gate reading them would pass on nothing")
	}
	return sites, asValues
}

func (c *auditCensus) doorSites() (sites []auditSite, asValues []string) {
	doors := c.doors()
	for _, door := range doors {
		key := auditFuncKey{dir: auditSeedDir, name: door.name}
		for _, at := range c.calls[door.name] {
			if at.src.dir != auditSeedDir && c.reaches(at, key) {
				sites = append(sites, auditSite{census: c, src: at.src, fn: at.fn, door: door, call: at.call})
			}
		}
	}
	for _, site := range c.sites {
		if !site.asValue || site.src.dir == auditSeedDir {
			continue
		}
		if slices.ContainsFunc(doors, func(door auditDoor) bool {
			return c.names(site.src, site.call.Fun, auditFuncKey{dir: auditSeedDir, name: door.name})
		}) {
			asValues = append(asValues, site.pos+" ("+site.opaque+")")
		}
	}
	return sites, asValues
}

// doors are storekit's exported writers that take their verb from the caller,
// as an action string or as an AuditTrail. A writer that fixes its own verb is
// held by storekit's refusal alone.
func (c *auditCensus) doors() []auditDoor {
	var doors []auditDoor
	for key, entity := range c.writers {
		decl := c.funcs[key]
		if key.dir != auditSeedDir || key.method || !ast.IsExported(key.name) {
			continue
		}
		door := auditDoor{
			name: key.name, verb: auditParamIndex(decl, "action"), entity: entity,
			before: auditParamIndex(decl, "before"), after: auditParamIndex(decl, "after"),
		}
		if door.verb < 0 {
			door.verb = auditParamOfType(decl, auditTrailType)
			door.verbIsTrail = door.verb >= 0
		}
		if door.verb >= 0 {
			doors = append(doors, door)
		}
	}
	slices.SortFunc(doors, func(a, b auditDoor) int { return strings.Compare(a.name, b.name) })
	return doors
}

// auditParamOfType is the position of fn's parameter declared with the named
// type, or -1.
func auditParamOfType(fn *ast.FuncDecl, typeName string) int {
	i := 0
	for _, field := range fn.Type.Params.List {
		for range max(1, len(field.Names)) {
			if types.ExprString(field.Type) == typeName {
				return i
			}
			i++
		}
	}
	return -1
}

// auditPackageStrings reads every package-level string a package declares, as
// the census resolves it, for a reader that needs one value per name.
func auditPackageStrings(c *auditCensus, dir string) map[string]string {
	out := map[string]string{}
	for name, constant := range c.consts[dir] {
		if values, ok := c.resolve(constant.src, nil, nil, constant.expr, 0); ok && len(values) == 1 {
			out[name] = values[0]
		}
	}
	return out
}

// plantedAuditCensus builds a census over planted sources alone, beside a
// storekit that declares the seed and one door of each shape. A falsification
// test reads its own writer through the same reader the gates use.
func plantedAuditCensus(t *testing.T, writer string) *auditCensus {
	t.Helper()
	const repo = "/planted"
	storekit := `package storekit
func insertAuditRows(ctx C, tx T, action, entityType string, rows []R) {}
func Audit(ctx C, tx T, action, entityType string, entityID I, before, after any) {
	insertAuditRows(ctx, tx, action, entityType, nil)
}
func AuditEvent(ctx C, tx T, action, entityType string, entityID I, after any) {
	insertAuditRows(ctx, tx, action, entityType, nil)
}`
	c := newAuditCensus(repo)
	for path, source := range map[string]string{
		auditSeedDir + "/doors.go": storekit,
		"backend/internal/planted/writer.go": "package planted\n\nimport \"" +
			auditModulePrefix + auditSeedDir + "\"\n\n" + writer,
	} {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), repo+"/"+path, source, 0)
		if err != nil {
			t.Fatalf("parsing the planted %s: %v", path, err)
		}
		c.index(repo+"/"+path, file)
	}
	if failed := c.build(); failed != "" {
		t.Fatal(failed)
	}
	return c
}
