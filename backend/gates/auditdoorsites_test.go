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
	sites, unjudged := census.doorSites()
	for _, key := range unjudged {
		t.Errorf("%s writes audit rows with a verb that reaches %s from no parameter and no constant, "+
			"so no gate judges its callers.\n\tPass the verb as a parameter, or teach doors() how this writer chooses it.",
			key, auditSeedFunc)
	}
	if len(sites) == 0 {
		t.Fatal("the audit census found no call into a storekit door, so every gate reading them would pass on nothing")
	}
	return sites
}

// doorSites are the calls into each door from outside the seed's package,
// and the exported writers whose verb no door rule could place.
func (c *auditEntityCensus) doorSites() (sites []auditDoorSite, unjudged []string) {
	seedPkg := c.seedPackage()
	doors, unjudged := c.doors()
	for _, door := range doors {
		for _, use := range c.index.uses[door.key] {
			if use.call != nil && use.pkg.path != seedPkg {
				sites = append(sites, auditDoorSite{door: door, use: use, index: c.index})
			}
		}
	}
	return sites, unjudged
}

func (c *auditEntityCensus) seedPackage() string {
	decl := c.index.decls[c.seed]
	if decl == nil {
		return ""
	}
	return c.index.declPkg[decl].path
}

// doors are the seed package's exported writers whose verb is a parameter
// the climb from the seed reaches. A writer that fixes its own verb is held by
// storekit's refusal alone. One whose verb the climb cannot read is a door
// only through an AuditTrail parameter, and is otherwise unjudged.
func (c *auditEntityCensus) doors() (doors []auditStoreDoor, unjudged []string) {
	seedPkg := c.seedPackage()
	params, fixed, unread := c.verbParams()
	for key := range c.writers {
		decl := c.index.decls[key]
		if decl == nil || decl.Recv != nil || !decl.Name.IsExported() || c.index.declPkg[decl].path != seedPkg {
			continue
		}
		info := c.index.declPkg[decl].info
		door := auditStoreDoor{
			key: key, name: decl.Name.Name, verb: -1, entity: auditParamIndex(info, decl, auditSeedParam),
			before: auditParamIndex(info, decl, "before"), after: auditParamIndex(info, decl, "after"),
		}
		if at, forwarded := params[key]; forwarded {
			door.verb = at
		} else if at := auditParamOfType(info, decl, auditTrailType); at >= 0 && unread[key] {
			door.verb, door.verbIsTrail = at, true
		}
		switch {
		case door.verb >= 0:
			doors = append(doors, door)
		case !fixed[key] || unread[key]:
			unjudged = append(unjudged, key)
		}
	}
	slices.SortFunc(doors, func(a, b auditStoreDoor) int { return strings.Compare(a.name, b.name) })
	slices.Sort(unjudged)
	return doors, unjudged
}

// verbParams climbs from the seed's action parameter through the seed's own
// package. It answers where each writer takes the verb as a parameter, which
// writers fix it, and which read it where the climb cannot follow.
func (c *auditEntityCensus) verbParams() (params map[string]int, fixed, unread map[string]bool) {
	params, fixed, unread = map[string]int{}, map[string]bool{}, map[string]bool{}
	decl := c.index.decls[c.seed]
	if decl == nil {
		return params, fixed, unread
	}
	seedPkg := c.seedPackage()
	queue := []auditDoor{{c.seed, auditParamIndex(c.index.declPkg[decl].info, decl, "action")}}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next.param < 0 {
			continue
		}
		for _, use := range c.index.uses[next.key] {
			if use.call == nil || use.decl == nil || use.pkg.path != seedPkg {
				continue
			}
			caller := auditFuncKey(use.pkg.info, use.decl)
			var read auditResolved
			ok := false
			if at := next.param + use.shift; at < len(use.call.Args) {
				read, ok = c.index.resolver(use).resolve(use.call.Args[at])
			}
			unread[caller] = unread[caller] || !ok
			fixed[caller] = fixed[caller] || len(read.values) > 0
			for _, forwarded := range read.params {
				if _, seen := params[forwarded.key]; !seen {
					params[forwarded.key] = forwarded.param
					queue = append(queue, forwarded)
				}
			}
		}
	}
	c.inheritVerbs(params, fixed, unread)
	return params, fixed, unread
}

// inheritVerbs gives a writer that hands only its record type to another
// writer that writer's verb: fixed or unread.
func (c *auditEntityCensus) inheritVerbs(params map[string]int, fixed, unread map[string]bool) {
	seedPkg := c.seedPackage()
	type edge struct{ caller, callee string }
	var edges []edge
	for callee := range c.writers {
		for _, use := range c.index.uses[callee] {
			if use.call != nil && use.decl != nil && use.pkg.path == seedPkg {
				edges = append(edges, edge{auditFuncKey(use.pkg.info, use.decl), callee})
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			if _, forwards := params[e.callee]; forwards {
				continue
			}
			if fixed[e.callee] && !fixed[e.caller] || unread[e.callee] && !unread[e.caller] {
				fixed[e.caller] = fixed[e.caller] || fixed[e.callee]
				unread[e.caller] = unread[e.caller] || unread[e.callee]
				changed = true
			}
		}
	}
}

// auditParamOfType is the position of decl's parameter declared with the
// named type or alias typeName, or -1.
func auditParamOfType(info *types.Info, decl *ast.FuncDecl, typeName string) int {
	obj, ok := info.Defs[decl.Name].(*types.Func)
	if !ok {
		return -1
	}
	params := obj.Signature().Params()
	for i := range params.Len() {
		switch declared := params.At(i).Type().(type) {
		case *types.Alias:
			if declared.Obj().Name() == typeName {
				return i
			}
		case *types.Named:
			if declared.Obj().Name() == typeName {
				return i
			}
		}
	}
	return -1
}
