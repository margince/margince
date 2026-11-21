// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The calls into storekit's audit doors, read from the census. A door is an
// exported writer in the seed's package whose verb its caller chooses: an
// `action` parameter, or an AuditTrail. A site is a call into one from
// outside that package.

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"
)

// auditTrailType is the verb type a door may take instead of an action
// string. Trail.Resolve admits update and restore only, so every call through
// such a door is update-shaped.
const auditTrailType = "AuditTrail"

// auditStoreDoor is one door and where its arguments sit. A position is -1
// when the door has no such parameter.
type auditStoreDoor struct {
	key, name                   string
	verb, entity, before, after int
	verbIsTrail                 bool
}

// auditDoorSite is one call into a door from outside storekit.
type auditDoorSite struct {
	door  auditStoreDoor
	use   auditFuncUse
	index *auditIndex
}

// key is path:function under the backend module, what a waiver is keyed by.
func (s auditDoorSite) key() string {
	return strings.TrimPrefix(auditWriterName(s.use), "backend/")
}

func (s auditDoorSite) argAt(i int) ast.Expr {
	at := i + s.use.shift
	if i < 0 || at >= len(s.use.call.Args) {
		return nil
	}
	return s.use.call.Args[at]
}

// values reads argument i to constants. A forwarded parameter is unknown
// here: its callers keep the guarantee, and a waiver says which.
func (s auditDoorSite) values(i int) ([]string, bool) {
	arg := s.argAt(i)
	if arg == nil {
		return nil, false
	}
	read, ok := s.index.resolver(s.use).resolve(arg)
	if !ok || len(read.params) > 0 || len(read.values) == 0 {
		return nil, false
	}
	return read.values, true
}

func (s auditDoorSite) verbs() ([]string, bool) {
	if s.door.verbIsTrail {
		return []string{"update"}, true
	}
	return s.values(s.door.verb)
}

// entityType is for a failure message only, so an unread one degrades to a
// description rather than costing a finding.
func (s auditDoorSite) entityType() string {
	if values, ok := s.values(s.door.entity); ok {
		return strings.Join(values, " or ")
	}
	return "an entity named at runtime"
}

// beforeIsAbsent reads the before argument the one way a call can show it:
// the untyped nil written there. A variable that holds nil reaches audit_log
// as SQL NULL too, and only storekit's refusal catches it.
func (s auditDoorSite) beforeIsAbsent() bool {
	arg := s.argAt(s.door.before)
	if arg == nil {
		return false
	}
	return s.use.pkg.info.Types[arg].IsNil()
}

// auditDoorSitesOf reads every door site of the module.
func auditDoorSitesOf(t *testing.T) []auditDoorSite {
	t.Helper()
	census := auditEntityCensusOf(t)
	if !census.seedInserts {
		t.Fatalf("%s no longer holds the INSERT into audit_log, so no door is derived from it", auditSeedFunc)
	}
	sites := census.doorSites()
	if len(sites) == 0 {
		t.Fatal("the audit census found no call into a storekit door, so every gate reading them would pass on nothing")
	}
	return sites
}

func (c *auditEntityCensus) doorSites() []auditDoorSite {
	var sites []auditDoorSite
	seedPkg := c.seedPackage()
	for _, door := range c.doors() {
		for _, use := range c.index.uses[door.key] {
			if use.call != nil && use.pkg.path != seedPkg {
				sites = append(sites, auditDoorSite{door: door, use: use, index: c.index})
			}
		}
	}
	return sites
}

func (c *auditEntityCensus) seedPackage() string {
	decl := c.index.decls[c.seed]
	if decl == nil {
		return ""
	}
	return c.index.declPkg[decl].path
}

// doors are the seed package's exported writers that take their verb from
// the caller. A writer that fixes its own verb is held by storekit's refusal
// alone.
func (c *auditEntityCensus) doors() []auditStoreDoor {
	seedPkg := c.seedPackage()
	var doors []auditStoreDoor
	for key := range c.writers {
		decl := c.index.decls[key]
		if decl == nil || decl.Recv != nil || !decl.Name.IsExported() || c.index.declPkg[decl].path != seedPkg {
			continue
		}
		info := c.index.declPkg[decl].info
		door := auditStoreDoor{
			key: key, name: decl.Name.Name,
			verb: auditParamIndex(info, decl, "action"), entity: auditParamIndex(info, decl, auditSeedParam),
			before: auditParamIndex(info, decl, "before"), after: auditParamIndex(info, decl, "after"),
		}
		if door.verb < 0 {
			door.verb = auditParamOfType(info, decl, auditTrailType)
			door.verbIsTrail = door.verb >= 0
		}
		if door.verb >= 0 {
			doors = append(doors, door)
		}
	}
	slices.SortFunc(doors, func(a, b auditStoreDoor) int { return strings.Compare(a.name, b.name) })
	return doors
}

// auditParamOfType is the position of decl's parameter whose named type is
// typeName, or -1.
func auditParamOfType(info *types.Info, decl *ast.FuncDecl, typeName string) int {
	obj, ok := info.Defs[decl.Name].(*types.Func)
	if !ok {
		return -1
	}
	params := obj.Signature().Params()
	for i := range params.Len() {
		if named, isNamed := types.Unalias(params.At(i).Type()).(*types.Named); isNamed && named.Obj().Name() == typeName {
			return i
		}
	}
	return -1
}
