// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The compliance read's filter, held as a census rather than as a list of the
// filters somebody remembered to test.
//
// A narrowing that is declared and not applied is the failure worth designing
// against: the read answers a WIDER question than the caller asked while
// looking exactly like an answer, and an auditor filtering to one actor sees
// the whole workspace and has no way to tell. storekit's own binding says the
// same thing about its half — "a filter cannot be published by one half and
// dropped by the other" — and this is that guarantee for a surface that takes
// typed contract parameters instead of a name=value map.
//
// Reflection over the STRUCT rather than a written-out list, because a list is
// what a new field is left off. A field added to AuditFilter and not applied
// fails here until somebody either applies it or says here why it narrows
// nothing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nonNarrowingFilterFields are the fields of AuditFilter that are not
// predicates, each with the reason. A field here is UNCHECKED by the census
// below, so the register is also the statement of what it does not cover — and
// an entry naming a field that no longer exists is reported, because a stale
// exemption is a field nobody is checking.
var nonNarrowingFilterFields = gatekit.Waive(map[string]string{
	"Limit": "page size, not a predicate: it bounds how many rows come back rather than which, and the statement's LIMIT is appended by the caller so the same $-numbering stays contiguous",
})

// narrowingClauses is the predicate each filter field must render — the COLUMN
// and the OPERATOR, not merely "something changed".
//
// Declared rather than derived, because deriving it from the code under test is
// how a check agrees with whatever the code does. `AND (a.actor_id <> $1 OR
// a.actor_id = $1)` differs from the empty clause and binds an argument, and
// narrows nothing; so does the right column with the wrong comparison, which is
// the shape that answers a window nobody asked for. A field with no entry here
// FAILS: a new filter has to say what it means before the census will pass it.
//
// The fragments are what the clause must CONTAIN, with the placeholder number
// left off because it depends on which other filters are set.
// gatekit:fixture the predicate each audit filter is required to render
var narrowingClauses = map[string]string{
	"Actor":      "a.actor_id = $",
	"EntityType": "a.entity_type = $",
	"EntityID":   "a.entity_id = $",
	"Action":     "a.action = $",
	// The window bounds are the pair a census cannot tell apart on its own:
	// swapped, they narrow as much and bind as many arguments, and answer
	// everything before Monday to somebody who asked what happened since.
	"From": "a.occurred_at >= $",
	"To":   "a.occurred_at <= $",
	// The keyset continues the newest-first ORDER BY the caller appends, so it
	// is a TUPLE comparison and strictly less-than: `<=` would re-serve the
	// last row of the previous page on every page.
	"Cursor": "(a.occurred_at, a.id) < ($",
}

// placeholderRe finds a bind placeholder in a rendered clause.
var placeholderRe = regexp.MustCompile(`\$(\d+)`)

// TestEveryAuditFilterFieldNarrowsTheRead is the census.
func TestEveryAuditFilterFieldNarrowsTheRead(t *testing.T) {
	t.Parallel()
	defer nonNarrowingFilterFields.AssertAllMatched(t)
	base, baseArgs, err := buildAuditWhere(AuditFilter{})
	if err != nil {
		t.Fatalf("an empty filter: %v", err)
	}
	if len(baseArgs) != 0 {
		t.Fatalf("an empty filter bound %d argument(s); it narrows nothing and should bind nothing", len(baseArgs))
	}

	shape := reflect.TypeOf(AuditFilter{})
	judged := 0
	for i := range shape.NumField() {
		name := shape.Field(i).Name
		if nonNarrowingFilterFields.Waived(t, name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			filter := AuditFilter{}
			set := reflect.ValueOf(&filter).Elem().Field(i)
			sample, ok := sampleFor(name, set.Type())
			if !ok {
				t.Fatalf("%s is a %s, which this census has no sample value for — add one, or the field is uncovered while the test still passes",
					name, set.Type())
			}
			set.Set(sample)

			where, args, err := buildAuditWhere(filter)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if where == base {
				t.Errorf("%s is set and the read is unchanged — it narrows nothing, so the caller is answered a wider question than they asked and cannot tell", name)
			}
			if len(args) == 0 {
				t.Errorf("%s narrowed the clause with no bound argument — a value spliced into SQL rather than bound", name)
			}
			want, declared := narrowingClauses[name]
			if !declared {
				t.Fatalf("%s narrows the read and narrowingClauses does not say how — a filter whose column and operator nobody declared is one a wrong comparison passes unnoticed", name)
			}
			if !strings.Contains(where, want) {
				t.Errorf("%s rendered %q, which does not carry %q — the clause differs from the empty one and still asks the wrong question", name, where, want)
			}
			assertPlaceholdersMatchArgs(t, where, args)
		})
		judged++
	}
	// A census that judged nothing certifies nothing. AuditFilter has never
	// had fewer than the six narrowing fields the contract publishes.
	for name := range narrowingClauses {
		if _, found := shape.FieldByName(name); !found {
			t.Errorf("narrowingClauses declares a predicate for %q, which AuditFilter no longer has — a stale declaration is a column nobody is checking", name)
		}
	}
	if judged < 6 {
		t.Fatalf("this census judged %d field(s) and expects at least 6 — the struct reader has stopped seeing fields rather than the filter having lost them", judged)
	}
}

// assertPlaceholdersMatchArgs holds what nothing else does: that the clause's
// placeholders and the argument slice agree.
//
// Postgres does not check this for us in any useful way — a statement short of
// arguments fails at execution with a message about a bind, and one carrying an
// extra binds it to nothing at all. The numbering is derived from the slice
// (`$` + len(args)) rather than typed, which is what makes it right; this is
// what says so.
func assertPlaceholdersMatchArgs(t *testing.T, where string, args []any) { //craft:ignore naked-any args are bind values on their way to pgx, whose encoding contract is any
	t.Helper()
	seen := map[int]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(where, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unreadable placeholder %q", m[0])
		}
		seen[n] = true
	}
	if len(seen) != len(args) {
		t.Errorf("the clause uses %d distinct placeholder(s) and %d argument(s) are bound: %q", len(seen), len(args), where)
	}
	for n := 1; n <= len(args); n++ {
		if !seen[n] {
			t.Errorf("argument %d is bound and $%d appears nowhere in %q — the arguments after it are read against the wrong placeholders", n, n, where)
		}
	}
}

// sampleFor builds a set value for one filter field. The CURSOR is minted
// rather than invented: buildAuditWhere decodes it, and any other string is a
// refusal rather than a narrowing.
func sampleFor(name string, fieldType reflect.Type) (reflect.Value, bool) {
	if fieldType.Kind() != reflect.Pointer {
		return reflect.Value{}, false
	}
	held := reflect.New(fieldType.Elem())
	switch value := held.Interface().(type) {
	case *string:
		*value = "sample"
		if name == "Cursor" {
			*value = sampleCursor()
		}
	case *ids.UUID:
		*value = ids.NewV7()
	case *time.Time:
		*value = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	case *int:
		*value = 5
	default:
		return reflect.Value{}, false
	}
	return held, true
}

// sampleCursor mints a token this read can decode. Any other string is a
// REFUSAL rather than a narrowing, which is what the cursor's own case below
// is about — so the census would be asserting the wrong thing with one.
func sampleCursor() string {
	return storekit.EncodeCursor(time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC), ids.NewV7())
}

// TestTheCursorIsTheOneFilterThatCanBeRefused — every other field narrows or
// does nothing, and this one can also be WRONG.
//
// It matters because the refusal is the caller's fault and has to read as one:
// a token this read cannot decode must stop the query rather than be dropped,
// which would answer the first page to somebody who asked for the fourth.
func TestTheCursorIsTheOneFilterThatCanBeRefused(t *testing.T) {
	t.Parallel()
	minted := sampleCursor()
	where, args, err := buildAuditWhere(AuditFilter{Cursor: &minted})
	if err != nil {
		t.Fatalf("a minted cursor was refused: %v", err)
	}
	if len(args) != 2 {
		t.Errorf("the keyset bound %d argument(s), want the occurred_at/id pair", len(args))
	}
	assertPlaceholdersMatchArgs(t, where, args)

	malformed := "not-a-cursor"
	if _, _, err := buildAuditWhere(AuditFilter{Cursor: &malformed}); err == nil {
		t.Error("an unreadable cursor was dropped rather than refused — the caller asked for a later page and would be handed the first")
	}
	// An EMPTY cursor is absent, not malformed: the handler passes the
	// parameter through whether or not the caller sent one.
	empty := ""
	if _, args, err := buildAuditWhere(AuditFilter{Cursor: &empty}); err != nil || len(args) != 0 {
		t.Errorf("an empty cursor bound %d argument(s) and answered %v; it means no cursor at all", len(args), err)
	}
}

// TestEveryPublishedAuditFilterReachesTheStore — the other half of the same
// guarantee, one level up.
//
// The census above holds that every field of AuditFilter narrows the read. It
// says nothing about a parameter the CONTRACT publishes and the handler never
// carries: the generated params struct grows when the OpenAPI document does,
// the mapping in ListAuditLog does not, and a filter that is documented,
// accepted and silently ignored is the widest answer of all — the caller has
// been told it works.
//
// Compared by NAME, because the two structs are written by different authors:
// one is generated from the contract and one is this module's own. The single
// spelling difference is the generator's (EntityId for EntityID), and it is
// declared rather than normalized away, so a second divergence has to be
// looked at instead of absorbed.
func TestEveryPublishedAuditFilterReachesTheStore(t *testing.T) {
	t.Parallel()
	// generatorSpellings maps a contract parameter onto the store field that
	// takes it where the two are not spelled identically.
	generatorSpellings := map[string]string{"EntityId": "EntityID"}

	published := reflect.TypeOf(crmcontracts.ListAuditLogParams{})
	stored := reflect.TypeOf(AuditFilter{})
	for i := range published.NumField() {
		name := published.Field(i).Name
		if mapped, renamed := generatorSpellings[name]; renamed {
			name = mapped
		}
		if _, carried := stored.FieldByName(name); !carried {
			t.Errorf("the contract publishes an audit-log filter %q that AuditFilter does not carry — a caller who sends it is answered the unnarrowed list and told nothing",
				published.Field(i).Name)
		}
	}
	// And the reverse, which is the cheaper mistake and still a mistake: a
	// store field nothing can reach is a narrowing no caller can ask for.
	for i := range stored.NumField() {
		name := stored.Field(i).Name
		for spelling, mapped := range generatorSpellings {
			if mapped == name {
				name = spelling
			}
		}
		if _, published := published.FieldByName(name); !published {
			t.Errorf("AuditFilter carries %q and the contract publishes no such parameter — the narrowing exists and nobody can ask for it", stored.Field(i).Name)
		}
	}
}

// TestTheHandlerCarriesEveryPublishedFilterIntoTheStore — the third link, and
// the one the two structural checks above leave open between them.
//
// Their chain is: the contract publishes a parameter, AuditFilter carries a
// field of that name, and every field of AuditFilter narrows the read. Nothing
// in it looks at the LINE that joins the first two. Delete `Action:
// params.Action` from ListAuditLog and both structs still carry the field, both
// checks still pass, and a documented, accepted filter is silently ignored —
// the widest answer of all, because the caller has been told it works.
//
// Read from the SOURCE rather than driven through the handler, because driving
// it proves one parameter at a time and this has to be a census: a field
// nobody wrote a case for is exactly the field that gets dropped. The handler
// binds in two shapes — a key in the AuditFilter literal, and an assignment to
// f.<Field> below it for the id that needs converting — so both count.
//
// And the VALUE has to come from the parameters, not merely exist. A field set
// to a constant is bound by every reading of the syntax and carries nothing the
// caller sent: `Action: addr("FIXED")` names the field, satisfies both structs,
// and drops the request exactly as deleting the line would. So the origin is
// what is checked — somewhere under the assigned expression, the params value
// this field takes.
func TestTheHandlerCarriesEveryPublishedFilterIntoTheStore(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "handlers.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the handler: %v", err)
	}
	handler := funcNamed(file, "ListAuditLog")
	if handler == nil {
		t.Fatal("privacy has no ListAuditLog handler — this census is reading a file that no longer holds the mapping")
	}
	bound := boundFilterFields(handler)

	shape := reflect.TypeOf(AuditFilter{})
	for i := range shape.NumField() {
		name := shape.Field(i).Name
		if !bound[name] {
			t.Errorf("ListAuditLog never carries %s out of its parameters — the contract publishes the filter, the store declares the field, and the request drops it in between: the caller is answered the unnarrowed list and told nothing",
				name)
		}
	}
	if len(bound) < shape.NumField() {
		t.Errorf("the handler binds %d of AuditFilter's %d fields", len(bound), shape.NumField())
	}
}

// boundFilterFields names the AuditFilter fields the handler sets from
// something, in either shape it uses.
func boundFilterFields(handler *ast.FuncDecl) map[string]bool {
	bound := map[string]bool{}
	// Locals that hold a parameter value, so an assignment routed through one
	// still counts as having come from the request.
	fromParams := map[string]bool{}
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if name, ok := node.Type.(*ast.Ident); !ok || name.Name != "AuditFilter" {
				return true
			}
			for _, element := range node.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok && readsRequestParams(pair.Value) {
					bound[key.Name] = true
				}
			}
		case *ast.AssignStmt:
			// The id needs converting, so it arrives through a local rather
			// than straight off params. The origin is still the question, one
			// hop further: the local has to be assigned from params somewhere
			// in the same function, which the walk records as it goes.
			for _, value := range node.Rhs {
				if readsRequestParams(value) {
					for _, target := range node.Lhs {
						if local, ok := target.(*ast.Ident); ok {
							fromParams[local.Name] = true
						}
					}
				}
			}
			for i, target := range node.Lhs {
				field, ok := target.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				receiver, ok := field.X.(*ast.Ident)
				if !ok || receiver.Name != "f" {
					continue
				}
				if i < len(node.Rhs) && (readsRequestParams(node.Rhs[i]) || readsLocalFromParams(node.Rhs[i], fromParams)) {
					bound[field.Sel.Name] = true
				}
			}
		}
		return true
	})
	return bound
}

// readsRequestParams reports whether an expression takes anything off the
// generated request parameters.
//
// By the RECEIVER's name, which is the handler's own — the parameter is
// declared `params crmcontracts.ListAuditLogParams`. A value that reaches for
// something else is a constant, a literal or a local, and none of those is the
// caller's.
func readsRequestParams(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if receiver, ok := sel.X.(*ast.Ident); ok && receiver.Name == "params" {
			found = true
		}
		return !found
	})
	return found
}

// readsLocalFromParams reports whether an expression reads a local this walk
// has already seen take its value from the parameters.
func readsLocalFromParams(expr ast.Expr, fromParams map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && fromParams[ident.Name] {
			found = true
		}
		return !found
	})
	return found
}

// funcNamed finds one top-level function in a parsed file.
func funcNamed(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}
