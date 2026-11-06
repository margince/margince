// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The grants a handler checks, read from the Go source. Admin against non-admin
// comes from here and from the seeded roles, never from a route's name.
//
// The walk starts at the method compose.Server serves an operation with. A check
// on the handler's main line is required. It is conditional when an if, switch or
// loop decides whether it runs, or when it is read as a boolean.

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	authPackagePath     = modulePath + "/internal/platform/auth"
	settingsPackagePath = modulePath + "/internal/platform/settings"
	configPackagePath   = modulePath + "/internal/platform/config"
	// adminRoleGrant and coachGrant spell the checks that name no object in their
	// arguments: auth.RequireAdmin and auth.RequireCoach.
	adminRoleGrant = "role:admin"
	coachGrant     = "team_lead:create"
	rbacMatrixPage = "../docs/reference/rbac-matrix.md"
	// unresolvedGrant marks an object or action only known at run time.
	unresolvedGrant = "?"
	// grantActionSeparator joins the actions of an auth.RequireAny.
	grantActionSeparator = "|"
)

// handlerGrants is what one handler checks.
type handlerGrants struct {
	required    []string
	conditional []string
}

// grantSource answers handlerGrants for any operation compose.Server serves.
type grantSource struct {
	graph   *importCallGraph
	server  *types.Named
	entries map[types.Object]string
	methods map[string]*types.Func
}

func loadGrantSource(t *testing.T) *grantSource {
	t.Helper()
	fset, files := parseGoFilesUnder(t, "internal")
	return grantSourceFrom(t, fset, files)
}

// grantSourceFrom type-checks parsed files, keyed by the directory each one names.
func grantSourceFrom(t *testing.T, fset *token.FileSet, files []*ast.File) *grantSource {
	t.Helper()
	byDir := map[string][]*ast.File{}
	for _, file := range files {
		// The generated contract adds no gate, and reading it costs most of the
		// type check. Its types stand in empty, as third-party imports do.
		name := filepath.ToSlash(fset.Position(file.Pos()).Filename)
		if !strings.HasSuffix(name, "_gen.go") {
			byDir[filepath.Dir(name)] = append(byDir[filepath.Dir(name)], file)
		}
	}
	graph := newImportCallGraph(fset, packagesInImportOrder(byDir))
	source := &grantSource{graph: graph, entries: settingEntryObjects(byDir, graph), methods: map[string]*types.Func{}}
	for _, pkg := range graph.packages {
		if pkg.Path() != composeImportPath {
			continue
		}
		named, ok := pkg.Scope().Lookup("Server").Type().(*types.Named)
		if !ok {
			t.Fatal("internal/compose declares no Server type, so no operation has a handler to read")
		}
		source.server = named
	}
	if source.server == nil {
		t.Fatal("the source walk found no internal/compose package")
	}
	for method := range types.NewMethodSet(source.server).Methods() {
		if fn, ok := method.Obj().(*types.Func); ok {
			source.methods[fn.Name()] = fn
		}
	}
	return source
}

// packagesInImportOrder puts each package after the ones it imports, so a checked
// package sees the real types of the module's other packages.
func packagesInImportOrder(byDir map[string][]*ast.File) [][]*ast.File {
	var ordered [][]*ast.File
	visited := map[string]bool{}
	var visit func(dir string)
	visit = func(dir string) {
		if visited[dir] {
			return
		}
		visited[dir] = true
		for _, file := range byDir[dir] {
			for _, spec := range file.Imports {
				if rest, ok := strings.CutPrefix(strings.Trim(spec.Path.Value, `"`), modulePath+"/"); ok {
					visit(rest)
				}
			}
		}
		if len(byDir[dir]) > 0 {
			ordered = append(ordered, byDir[dir])
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		visit(dir)
	}
	return ordered
}

// settingEntryObjects maps each package-level stored setting to the RBAC object
// it declares. platform/settings checks that object at run time, so a walk that
// did not read the declaration would see an unknown grant on every setting.
func settingEntryObjects(byDir map[string][]*ast.File, graph *importCallGraph) map[types.Object]string {
	out := map[types.Object]string{}
	for _, files := range byDir {
		for _, file := range files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.VAR {
					continue
				}
				collectSettingEntries(gen, graph.fileInfos[file], out)
			}
		}
	}
	return out
}

func collectSettingEntries(gen *ast.GenDecl, info *types.Info, out map[types.Object]string) {
	if info == nil {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != len(value.Values) {
			continue
		}
		for i, name := range value.Names {
			if object := declaredSettingObject(value.Values[i], info); object != "" {
				out[info.Defs[name]] = object
			}
		}
	}
}

// declaredSettingObject reads the object out of settings.Define(key, object, …)
// or config.DefineStored(spec, config.Store{RBACObject: object}), through any
// builder calls chained onto either.
func declaredSettingObject(expr ast.Expr, info *types.Info) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	if chained, isChained := call.Fun.(*ast.SelectorExpr); isChained {
		if _, onCall := chained.X.(*ast.CallExpr); onCall {
			return declaredSettingObject(chained.X, info)
		}
	}
	fn := calledFunc(call, info)
	switch {
	case fn == nil || fn.Pkg() == nil:
		return ""
	case fn.Pkg().Path() == settingsPackagePath && fn.Name() == "Define" && len(call.Args) > 1:
		return constantText(call.Args[1], info, nil)
	case fn.Pkg().Path() == configPackagePath && fn.Name() == "DefineStored" && len(call.Args) > 1:
		store, isLiteral := call.Args[1].(*ast.CompositeLit)
		if !isLiteral {
			return ""
		}
		for _, element := range store.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			if !isField {
				continue
			}
			if key, isKey := field.Key.(*ast.Ident); isKey && key.Name == "RBACObject" {
				return constantText(field.Value, info, nil)
			}
		}
	}
	return ""
}

// calleeIdent is the name a call goes by, generic or not.
func calleeIdent(call *ast.CallExpr) *ast.Ident {
	fun := call.Fun
	switch indexed := fun.(type) {
	case *ast.IndexExpr:
		fun = indexed.X
	case *ast.IndexListExpr:
		fun = indexed.X
	}
	switch named := fun.(type) {
	case *ast.Ident:
		return named
	case *ast.SelectorExpr:
		return named.Sel
	}
	return nil
}

// calledFunc is the declared function a call names.
func calledFunc(call *ast.CallExpr, info *types.Info) *types.Func {
	ident := calleeIdent(call)
	if ident == nil {
		return nil
	}
	used, ok := info.Uses[ident].(*types.Func)
	if !ok {
		return nil
	}
	return used.Origin()
}

// constantText is the string a constant, a literal or a bound parameter holds, or
// empty when only the running program knows it.
func constantText(expr ast.Expr, info *types.Info, bound map[types.Object]string) string {
	var ident *ast.Ident
	switch x := expr.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return ""
		}
		text, err := strconv.Unquote(x.Value)
		if err != nil {
			return ""
		}
		return text
	case *ast.Ident:
		ident = x
	case *ast.SelectorExpr:
		ident = x.Sel
	default:
		return ""
	}
	switch obj := info.Uses[ident].(type) {
	case *types.Const:
		if obj.Val().Kind() == constant.String {
			return constant.StringVal(obj.Val())
		}
	case *types.Var:
		return bound[obj]
	}
	return ""
}

// handlerName is the Go method oapi-codegen names for an operationId.
func handlerName(operationID string) string {
	var name strings.Builder
	for part := range strings.SplitSeq(operationID, "_") {
		if part != "" {
			name.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return name.String()
}

// grantsOf walks the handler compose.Server serves an operation with. The bool is
// false when Server serves no such method. The compiler holds Server to every
// operation, so a caller treats that as a broken walk.
func (s *grantSource) grantsOf(operationID string) (handlerGrants, bool) {
	fn, ok := s.methods[handlerName(operationID)]
	if !ok {
		return handlerGrants{}, false
	}
	walk := &grantWalk{source: s, seen: map[string]bool{}}
	walk.function(fn, nil, true)
	slices.Sort(walk.required)
	slices.Sort(walk.conditional)
	return handlerGrants{required: slices.Compact(walk.required), conditional: slices.Compact(walk.conditional)}, true
}

type grantWalk struct {
	source      *grantSource
	seen        map[string]bool
	required    []string
	conditional []string
	// entryObject is the object of the last stored setting the walk met, for the
	// checks platform/settings makes on it.
	entryObject string
}

func (w *grantWalk) function(fn *types.Func, bound map[types.Object]string, mainLine bool) {
	decl := w.source.graph.decls[fn.Origin()]
	if decl == nil {
		return
	}
	key := w.visitKey(fn, bound, mainLine)
	if w.seen[key] {
		return
	}
	w.seen[key] = true
	w.statements(decl.Body.List, w.source.graph.infos[decl], bound, mainLine)
}

// visitKey keeps each bound value at its parameter's position, so a helper called
// with two objects swapped is read a second time.
func (w *grantWalk) visitKey(fn *types.Func, bound map[types.Object]string, mainLine bool) string {
	parts := []string{fn.FullName(), strconv.FormatBool(mainLine), w.entryObject}
	if sig, ok := fn.Type().(*types.Signature); ok {
		for param := range sig.Params().Variables() {
			parts = append(parts, bound[param])
		}
	}
	return strings.Join(parts, "|")
}

func (w *grantWalk) statements(list []ast.Stmt, info *types.Info, bound map[types.Object]string, mainLine bool) {
	for _, stmt := range list {
		w.statement(stmt, info, bound, mainLine)
	}
}

// statement keeps a call on the main line only where every run reaches it. That
// holds for the init and condition of an if, and never for its arms or a loop body.
func (w *grantWalk) statement(stmt ast.Stmt, info *types.Info, bound map[types.Object]string, mainLine bool) {
	switch s := stmt.(type) {
	case nil:
	case *ast.BlockStmt:
		w.statements(s.List, info, bound, mainLine)
	case *ast.IfStmt:
		w.statement(s.Init, info, bound, mainLine)
		w.expression(s.Cond, info, bound, mainLine)
		w.statements(s.Body.List, info, bound, false)
		w.statement(s.Else, info, bound, false)
	case *ast.SwitchStmt:
		w.statement(s.Init, info, bound, mainLine)
		w.expression(s.Tag, info, bound, mainLine)
		w.statements(s.Body.List, info, bound, false)
	case *ast.TypeSwitchStmt:
		w.statement(s.Init, info, bound, mainLine)
		w.statements(s.Body.List, info, bound, false)
	case *ast.CaseClause:
		w.statements(s.Body, info, bound, false)
	case *ast.CommClause:
		w.statements(s.Body, info, bound, false)
	case *ast.SelectStmt:
		w.statements(s.Body.List, info, bound, false)
	case *ast.ForStmt:
		w.statements(s.Body.List, info, bound, false)
	case *ast.RangeStmt:
		w.expression(s.X, info, bound, mainLine)
		w.statements(s.Body.List, info, bound, false)
	case *ast.LabeledStmt:
		w.statement(s.Stmt, info, bound, mainLine)
	default:
		ast.Inspect(stmt, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			w.expression(expr, info, bound, mainLine)
			return false
		})
	}
}

func (w *grantWalk) expression(expr ast.Expr, info *types.Info, bound map[types.Object]string, mainLine bool) {
	if expr == nil {
		return
	}
	called := map[*ast.Ident]bool{}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			w.statements(x.Body.List, info, bound, mainLine)
			return false
		case *ast.BinaryExpr:
			w.expression(x.X, info, bound, false)
			w.expression(x.Y, info, bound, false)
			return false
		case *ast.CallExpr:
			if ident := calleeIdent(x); ident != nil {
				called[ident] = true
			}
			w.noteSettingEntries(x, info)
			w.call(x, info, bound, mainLine)
		case *ast.Ident:
			if object, isEntry := w.source.entries[info.Uses[x]]; isEntry {
				w.entryObject = object
			}
			w.methodValue(x, called, info, mainLine)
		}
		return true
	})
}

// noteSettingEntries reads a call's arguments and receiver before the walk enters
// the callee, because the check inside it asks the entry for its object.
func (w *grantWalk) noteSettingEntries(call *ast.CallExpr, info *types.Info) {
	ast.Inspect(call, func(n ast.Node) bool {
		if _, isClosure := n.(*ast.FuncLit); isClosure {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok {
			if object, isEntry := w.source.entries[info.Uses[ident]]; isEntry {
				w.entryObject = object
			}
		}
		return true
	})
}

// methodValue follows a function handed on rather than called, as
// h.moveRole(w, r, key, h.svc.ArchiveRole) does.
func (w *grantWalk) methodValue(ident *ast.Ident, called map[*ast.Ident]bool, info *types.Info, mainLine bool) {
	used, ok := info.Uses[ident].(*types.Func)
	if !ok || called[ident] {
		return
	}
	for _, callee := range w.source.graph.callees(used.Origin()) {
		w.function(callee, nil, mainLine)
	}
}

func (w *grantWalk) call(call *ast.CallExpr, info *types.Info, bound map[types.Object]string, mainLine bool) {
	used := calledFunc(call, info)
	if used == nil {
		return
	}
	if used.Pkg() != nil && used.Pkg().Path() == authPackagePath && refusingChecks[used.Name()] {
		w.recordRefusal(used.Name(), call, info, bound, mainLine)
		return
	}
	// The primitive every object grant ends at. It answers a boolean and refuses
	// nobody, so it is conditional however it is called.
	if isPermissionsAllows(used) {
		if len(call.Args) > 1 {
			w.conditional = append(w.conditional, w.objectGrant(call.Args[0], call.Args[1:], info, bound))
		}
		return
	}
	if isAuthAllows(used) {
		w.recordRefusal(used.Name(), call, info, bound, false)
		return
	}
	for _, callee := range w.source.graph.callees(used) {
		w.function(callee, boundArguments(callee, call, info, bound), mainLine)
	}
}

func isAuthAllows(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil && fn.Pkg() != nil && fn.Pkg().Path() == authPackagePath && fn.Name() == authAllows
}

// refusingChecks are the auth functions the walk records by name, because each one
// refuses by itself. Every other auth function is walked through to these or to
// isPermissionsAllows, so a new wrapper cannot hide a grant.
var refusingChecks = map[string]bool{"Require": true, "RequireAny": true, "RequireAdmin": true, "RequireCoach": true}

// authAllows is Require read as a boolean, so the walk records it by name as a
// conditional check. Its object and action reach the primitive inside a Check,
// where the walk cannot bind them.
const authAllows = "Allows"

func isPermissionsAllows(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || fn.Name() != "Allows" {
		return false
	}
	named, isNamed := types.Unalias(sig.Recv().Type()).(*types.Named)
	return isNamed && named.Obj().Name() == "Permissions"
}

func (w *grantWalk) recordRefusal(name string, call *ast.CallExpr, info *types.Info, bound map[types.Object]string, mainLine bool) {
	grant := ""
	switch name {
	case "RequireAdmin":
		grant = adminRoleGrant
	case "RequireCoach":
		grant = coachGrant
	default:
		if len(call.Args) < 3 {
			return
		}
		grant = w.objectGrant(call.Args[1], call.Args[2:], info, bound)
	}
	if mainLine {
		w.required = append(w.required, grant)
		return
	}
	w.conditional = append(w.conditional, grant)
}

func (w *grantWalk) objectGrant(objectArg ast.Expr, actionArgs []ast.Expr, info *types.Info, bound map[types.Object]string) string {
	object := constantText(objectArg, info, bound)
	if object == "" {
		object = w.entryObject
	}
	actions := make([]string, 0, len(actionArgs))
	for _, arg := range actionArgs {
		actions = append(actions, constantText(arg, info, bound))
	}
	slices.Sort(actions)
	return grantText(object, strings.Join(actions, grantActionSeparator))
}

func grantText(object, action string) string {
	if object == "" {
		object = unresolvedGrant
	}
	if action == "" || strings.HasPrefix(action, grantActionSeparator) {
		action = unresolvedGrant
	}
	return object + ":" + action
}

// boundArguments carries the constants a caller passes, so a helper that takes
// the object as a parameter still names it.
func boundArguments(callee *types.Func, call *ast.CallExpr, info *types.Info, bound map[types.Object]string) map[types.Object]string {
	sig, ok := callee.Type().(*types.Signature)
	if !ok {
		return nil
	}
	out := map[types.Object]string{}
	for i := range min(sig.Params().Len(), len(call.Args)) {
		if text := constantText(call.Args[i], info, bound); text != "" {
			out[sig.Params().At(i)] = text
		}
	}
	return out
}

// roleGrants is the seeded role matrix, one CRUD cell per role and object.
type roleGrants map[string]map[string]string

var (
	rbacMatrixHead = regexp.MustCompile(`(?m)^\| Object \|(.*)\|\s*$`)
	rbacMatrixRow  = regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|(.*)\\|\\s*$")
)

// readRoleGrants reads the published matrix. identity's own test holds that page
// to the documents a new workspace is seeded with. Those documents live in a package
// internal to identity, which this gate cannot import.
func readRoleGrants(t *testing.T) roleGrants {
	t.Helper()
	body, err := os.ReadFile(rbacMatrixPage)
	if err != nil {
		t.Fatalf("reading the seeded role matrix: %v", err)
	}
	head := rbacMatrixHead.FindStringSubmatch(string(body))
	if head == nil {
		t.Fatalf("%s has no matrix header, so no grant can be judged", rbacMatrixPage)
	}
	roles := tableCells(head[1])
	out := roleGrants{}
	for _, row := range rbacMatrixRow.FindAllStringSubmatch(string(body), -1) {
		cells := tableCells(row[2])
		if len(cells) != len(roles) {
			continue
		}
		out[row[1]] = map[string]string{}
		for i, role := range roles {
			out[row[1]][role] = cells[i]
		}
	}
	return out
}

func tableCells(row string) []string {
	var cells []string
	for cell := range strings.SplitSeq(row, "|") {
		cells = append(cells, strings.Trim(strings.TrimSpace(cell), "`"))
	}
	return cells
}

// grantActionPosition is where an action sits in a CRUD cell of the matrix.
var grantActionPosition = map[string]int{"create": 0, "read": 1, "update": 2, "delete": 3}

// holds answers whether a role holds one grant. known is false for an unresolved
// grant or an object the matrix does not list.
func (g roleGrants) holds(role, grant string) (held, known bool) {
	if grant == adminRoleGrant {
		return role == "admin", true
	}
	object, actions, ok := strings.Cut(grant, ":")
	cell, listed := g[object][role]
	if !ok || !listed {
		return false, false
	}
	for action := range strings.SplitSeq(actions, grantActionSeparator) {
		at, isAction := grantActionPosition[action]
		if !isAction || at >= len(cell) {
			return false, false
		}
		if cell[at] != '-' {
			return true, true
		}
	}
	return false, true
}

// plantedAuth keeps the shapes of platform/auth that decide what a walk reads. They
// are a refusal, its boolean form, a wrapper with no object of its own, and the primitive.
const plantedAuth = `package auth

import "context"

type Permissions struct{}

func (Permissions) Allows(object, action string) bool { return true }

type Principal struct{ Permissions Permissions }

func actor(ctx context.Context) Principal { return Principal{} }

func Require(ctx context.Context, object, action string) error { return nil }

func Allows(ctx context.Context, object, action string) bool {
	return Require(ctx, object, action) == nil
}

func ReadGranted(ctx context.Context, object string) bool {
	return actor(ctx).Permissions.Allows(object, "read")
}

func RequireThing(ctx context.Context) error { return Require(ctx, "thing", "update") }
`

const plantedServer = `package compose

import (
	"context"

	"` + authPackagePath + `"
)

type Server struct{}

func check(ctx context.Context, object, other string) error { return auth.Require(ctx, object, "read") }

func (Server) Swaps(ctx context.Context) error {
	if err := check(ctx, "deal", "lead"); err != nil {
		return err
	}
	return check(ctx, "lead", "deal")
}

func (Server) Refuses(ctx context.Context) error { return auth.Require(ctx, "deal", "read") }

func (Server) Asks(ctx context.Context) error {
	if !auth.Allows(ctx, "deal", "update") {
		return nil
	}
	return nil
}

func (Server) Stores(ctx context.Context) bool { return auth.Allows(ctx, "deal", "delete") }

func (Server) Masks(ctx context.Context) bool { return auth.ReadGranted(ctx, "forecast") }

func (Server) Wraps(ctx context.Context) error { return auth.RequireThing(ctx) }

func (Server) Direct(ctx context.Context, p auth.Principal) bool { return p.Permissions.Allows("role_admin", "read") }
`

// A boolean read of a grant refuses nobody by itself, so it is conditional wherever
// it stands. An auth function that wraps a check is read through.
func TestTheGrantWalkReadsEveryFormOfACheck(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var files []*ast.File
	for name, source := range map[string]string{
		"internal/platform/auth/auth.go": plantedAuth,
		"internal/compose/server.go":     plantedServer,
	} {
		file, err := parser.ParseFile(fset, name, source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files = append(files, file)
	}
	source := grantSourceFrom(t, fset, files)
	for _, tc := range []struct {
		handler     string
		required    []string
		conditional []string
	}{
		{handler: "refuses", required: []string{"deal:read"}},
		{handler: "asks", conditional: []string{"deal:update"}},
		{handler: "stores", conditional: []string{"deal:delete"}},
		{handler: "masks", conditional: []string{"forecast:read"}},
		{handler: "wraps", required: []string{"thing:update"}},
		{handler: "direct", conditional: []string{"role_admin:read"}},
		{handler: "swaps", required: []string{"deal:read", "lead:read"}},
	} {
		grants, served := source.grantsOf(tc.handler)
		if !served {
			t.Errorf("the planted Server serves no %s", tc.handler)
			continue
		}
		if !slices.Equal(grants.required, tc.required) || !slices.Equal(grants.conditional, tc.conditional) {
			t.Errorf("%s: required %v and conditional %v, want %v and %v", tc.handler, grants.required, grants.conditional, tc.required, tc.conditional)
		}
	}
}
