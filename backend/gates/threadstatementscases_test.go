// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestAnAvailabilityGuardDoesNotCoverASiblingQuery(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func read(tx DB,ctx Context) { tx.Query(ctx, `SELECT id FROM activity a WHERE activity_thread_available(a.thread_key)`); query:=`SELECT id FROM activity a WHERE a.id=$1`; tx.QueryRow(ctx,query) }", 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	offenders := pkg.unguardedQueries(pkg.functions["read"])
	if len(offenders) != 1 || offenders[0] != "SELECT id FROM activity a WHERE a.id=$1" {
		t.Fatalf("unguarded sibling queries: %v", offenders)
	}
}

func TestAConditionalAvailabilityClauseCannotCoverTheOtherBranch(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func read(tx DB,ctx Context,include bool){query:=`SELECT id FROM activity a`;if include{query=`SELECT id FROM activity a WHERE activity_thread_available(a.thread_key)`};tx.Query(ctx,query)}", 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 1 {
		t.Fatalf("unguarded branch queries=%v", got)
	}
}

func TestSQLExpansionCannotDropAnUnguardedTail(t *testing.T) {
	t.Parallel()
	var source strings.Builder
	source.WriteString("package fixture; func read(tx DB, ctx Context, choice int) { query := `SELECT id FROM activity a WHERE activity_thread_available(a.thread_key)`;")
	for index := range 300 {
		source.WriteString("if choice == " + strconv.Itoa(index) + " { query = `SELECT id FROM activity a WHERE activity_thread_available(a.thread_key) AND a.id = $" + strconv.Itoa(index+1) + "` };")
	}
	const unguarded = "SELECT id FROM activity a WHERE a.id=$1"
	source.WriteString("if choice < 0 { query = `" + unguarded + "` }; tx.Query(ctx,query) }")
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 1 || got[0] != unguarded {
		t.Fatalf("unguarded tail = %v", got)
	}
}

func TestSQLExpansionOverflowFailsInsteadOfShorteningItsCensus(t *testing.T) {
	t.Parallel()
	alternatives := make([]threadSQLValue, threadSQLMaxAlternatives+1)
	for index := range alternatives {
		alternatives[index].text = strconv.Itoa(index)
	}
	values := joinThreadSQL(alternatives, []threadSQLValue{{}})
	if len(values) != 1 || !values[0].unresolved {
		t.Fatalf("overflow = %v", values)
	}
}

func TestSQLExpansionDepthFailsInsteadOfHidingItsQuery(t *testing.T) {
	t.Parallel()
	var expr ast.Expr = &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("SELECT id FROM activity")}
	for range threadSQLMaxDepth + 1 {
		expr = &ast.ParenExpr{X: expr}
	}
	values := (threadSQLPackage{}).values(expr, nil, map[string]bool{}, nil, 0)
	if len(values) != 1 || !values[0].unresolved {
		t.Fatalf("deep query = %v", values)
	}
}

func TestAMethodSQLBuilderCannotHideAnUnguardedQuery(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func(s Store) sql() string {return `SELECT id FROM activity a`};func(s Store) read(tx DB,ctx Context){tx.Query(ctx,s.sql())}", 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["Store.read"]); len(got) != 1 {
		t.Fatalf("method SQL findings=%v", got)
	}
}

func TestABatchedQueryNeedsItsOwnAvailabilityBoundary(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture;func read(batch Batch,query string){batch.Queue(`SELECT id FROM activity a`,1)}", 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 1 {
		t.Fatalf("batch SQL findings=%v", got)
	}
}

func TestEachActivityAliasNeedsItsOwnPositiveBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		statement string
		guarded   bool
	}{
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key)`, true},
		{`SELECT a.id FROM activity a JOIN activity b ON b.id<>a.id WHERE activity_thread_available(a.thread_key)`, false},
		{`SELECT a.id FROM activity a WHERE EXISTS(SELECT 1 FROM activity b WHERE activity_thread_available(b.thread_key))`, false},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) OR a.id=$1`, false},
		{`SELECT a.id FROM activity a WHERE NOT(activity_thread_available(a.thread_key))`, false},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) AND NOT EXISTS(SELECT 1 FROM activity b WHERE activity_thread_available(b.thread_key))`, false},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) AND NOT EXISTS(SELECT 1 FROM activity b WHERE b.id=a.id AND (NOT activity_thread_available(b.thread_key) OR b.is_done))`, true},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) AND NOT(EXISTS(SELECT 1 FROM activity b WHERE activity_thread_available(b.thread_key)))`, false},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) UNION ALL SELECT a.id FROM activity a`, false},
		{`SELECT id FROM activity WHERE activity_thread_available(thread_key)`, true},
	}
	for _, tc := range cases {
		if got := threadStatementGuarded(tc.statement); got != tc.guarded {
			t.Errorf("guarded=%t, want %t: %s", got, tc.guarded, tc.statement)
		}
	}
}

func TestAClauseBuilderCannotCoverAnotherAliasOrNegativePosition(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"package fixture;func read(tx DB,ctx Context){batch.Queue(`SELECT id FROM activity a`)}",
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,`SELECT a.id FROM activity a WHERE `+auth.ActivityAvailableClause(`b`))}",
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,`SELECT a.id FROM activity a WHERE NOT (`+auth.ActivityAvailableClause(`a`)+`)`)}",
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,fmt.Sprintf(`SELECT a.id FROM activity a WHERE %s OR a.id=$1`,auth.ActivityAvailableClause(`a`)))}",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		pkg := threadSQLPackageOf([]*ast.File{file})
		if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 1 {
			t.Errorf("findings=%v for %s", got, source)
		}
	}
}

func TestOpaqueSQLAndLocalExecutionWrappersCannotShortenTheCensus(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"package fixture;func read(tx DB,ctx Context,q Query){tx.Query(ctx,q.SQL)}",
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,external.SQL())}",
		"package fixture;func(s Store)update(ctx Context,sql string){s.tx.Exec(ctx,sql)};func(s Store)read(ctx Context){s.update(ctx,`SELECT id FROM activity a`)}",
		"package fixture;func read(tx DB,ctx Context){q:=Query{SQL:`SELECT id FROM activity a`};tx.Query(ctx,q.SQL)}",
		"package fixture;func read(tx DB,ctx Context,selection string){tx.Query(ctx,`SELECT id `+selection)};func caller(){read(nil,nil,`FROM activity a`)}",
		"package fixture;type Spec struct{Odd string};func read(tx DB,ctx Context,spec Spec){tx.Query(ctx,`SELECT id `+spec.Odd)};func caller(){read(nil,nil,Spec{Odd:`FROM activity a`})}",
		"package fixture;func read(tx DB,ctx Context,selection string){tx.Query(ctx,`SELECT id `+selection)};func wrapper(tx DB,ctx Context,selection string){read(tx,ctx,selection)};func caller(){wrapper(nil,nil,`FROM activity a`)}",
		"package fixture;func read(tx DB,ctx Context){var q strings.Builder;q.WriteString(`SELECT id FROM activity a`);tx.Query(ctx,q.String())}",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		pkg := threadSQLPackageOf([]*ast.File{file})
		fn := pkg.functions["read"]
		if fn == nil {
			fn = pkg.functions["Store.read"]
		}
		if got := pkg.unguardedQueries(fn); len(got) != 1 {
			t.Errorf("findings=%v for %s", got, source)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture;func read(tx DB,ctx Context,selection string){tx.Query(ctx,`SELECT id `+selection)};func caller(){read(nil,nil,`FROM company`)}", 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 0 {
		t.Fatalf("closed non-activity selection entered the activity census: %v", got)
	}

}

func TestNullableJoinAliasesCannotGuardThePreservedActivity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		statement string
		guarded   bool
	}{
		{`SELECT a.subject FROM activity a LEFT JOIN activity b ON b.id=a.id AND b.kind='note' WHERE activity_thread_available(b.thread_key)`, false},
		{`SELECT a.subject FROM activity a LEFT JOIN contact c ON c.id=a.id AND activity_thread_available(a.thread_key)`, false},
		{`SELECT a.subject FROM contact c RIGHT JOIN activity a ON c.id=a.id AND activity_thread_available(a.thread_key)`, false},
		{`SELECT a.subject FROM activity a FULL JOIN contact c ON c.id=a.id AND activity_thread_available(a.thread_key)`, false},
		{`SELECT a.subject FROM activity a LEFT JOIN activity b ON b.id=a.id WHERE b.id=a.id AND activity_thread_available(b.thread_key)`, true},
		{`SELECT a.subject FROM activity a JOIN activity b ON b.id=a.id WHERE activity_thread_available(b.thread_key)`, true},
		{`SELECT a.subject FROM activity a RIGHT JOIN contact c ON c.id=a.id AND activity_thread_available(a.thread_key)`, true},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) AND NOT EXISTS(SELECT 1 FROM activity b WHERE b.thread_key=a.thread_key AND activity_thread_available(b.thread_key))`, true},
		{`SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key) AND NOT EXISTS(SELECT 1 FROM activity b WHERE b.source_activity_id=a.id AND activity_thread_available(b.thread_key))`, false},
	} {
		if got := threadStatementGuarded(tc.statement); got != tc.guarded {
			t.Errorf("guarded=%t,want%t: %s", got, tc.guarded, tc.statement)
		}
	}
}

func TestARepairPendingPredicateCannotAdmitAnOrdinaryActivityRead(t *testing.T) {
	t.Parallel()
	source := "package fixture;import auth \"github.com/margince/margince/backend/internal/platform/auth\";func read(tx DB,ctx Context,arg Args){pending,err:=auth.ActivityRepairPendingClause(ctx,`a`,arg);if err!=nil{return};tx.Query(ctx,`SELECT a.subject FROM activity a WHERE `+pending)}"
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) != 1 {
		t.Fatalf("ordinary read admitted pending predicate: %v", got)
	}
}

func TestAnApprovedSQLVariantSetCannotCoverANewUnsafeSibling(t *testing.T) {
	t.Parallel()
	existing := []string{`SELECT activity_thread_available(thread_key) FROM activity WHERE id=@id`}
	original, complete := threadStatementSetFingerprint(existing)
	if !complete || original == "" {
		t.Fatal("literal statement set was not resolved")
	}
	changed, complete := threadStatementSetFingerprint(append(existing, `SELECT subject FROM activity WHERE id=@id`))
	if !complete || original == changed {
		t.Fatal("new unsafe sibling retained the approved variant fingerprint")
	}
	if _, complete := threadStatementSetFingerprint([]string{`SELECT subject FROM activity WHERE ?`}); complete {
		t.Fatal("opaque predicate was declared fully expanded")
	}
	if _, complete := threadStatementSetFingerprint([]string{`SELECT subject FROM activity WHERE id=$?`}); !complete {
		t.Fatal("a numeric bind slot was mistaken for an opaque SQL fragment")
	}
}

func TestProjectionGuardsAndCTEAliasReuseDoNotExcludeOuterRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		statement string
		guarded   bool
	}{
		{`WITH x AS(SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key)) SELECT a.subject FROM activity a LEFT JOIN x ON x.id=a.id`, false},
		{`WITH x AS(SELECT a.id FROM activity a WHERE activity_thread_available(a.thread_key)) SELECT a.subject FROM activity a JOIN x ON x.id=a.id`, true},
		{`SELECT count(*) FILTER(WHERE activity_thread_available(a.thread_key)) FROM activity a`, false},
		{`SELECT a.subject FROM activity a WHERE a.is_done BETWEEN false AND activity_thread_available(a.thread_key)`, false},
		{`SELECT a.subject FROM activity a WHERE a.id=CASE WHEN a.kind='email' OR a.kind='message' THEN $1 ELSE $2 END AND activity_thread_available(a.thread_key)`, true},
	} {
		if got := threadStatementGuarded(tc.statement); got != tc.guarded {
			t.Errorf("guarded=%t,want%t: %s", got, tc.guarded, tc.statement)
		}
	}
}

func TestQuotedActivityReadsAndSanitizedTableIdentifiersStayInTheCensus(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,`SELECT subject FROM \"activity\"`)}",
		"package fixture;func read(tx DB,ctx Context){tx.Query(ctx,`SELECT subject FROM public.\"activity\"`)}",
		"package fixture;import pgx \"github.com/jackc/pgx/v5\";func read(tx DB,ctx Context,name string){tx.Query(ctx,`SELECT subject FROM `+pgx.Identifier{name}.Sanitize())}",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		pkg := threadSQLPackageOf([]*ast.File{file})
		if got := pkg.unguardedQueries(pkg.functions["read"]); len(got) == 0 {
			t.Errorf("activity target escaped: %s", source)
		}
	}
}

func TestNegativeExistenceCannotFabricateAbsenceFromUnavailableRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		sql     string
		guarded bool
	}{
		{`SELECT NOT EXISTS(SELECT 1 FROM activity answer_mail WHERE activity_thread_available(answer_mail.thread_key))`, false},
		{`SELECT NOT EXISTS(SELECT 1 FROM activity answer_mail WHERE NOT activity_thread_available(answer_mail.thread_key))`, true},
		{`SELECT EXISTS(SELECT 1 FROM activity answer_mail WHERE NOT activity_thread_available(answer_mail.thread_key))`, false},
		{`SELECT subject FROM activity answer_mail WHERE NOT activity_thread_available(answer_mail.thread_key)`, false},
	} {
		if got := threadStatementGuarded(tc.sql); got != tc.guarded {
			t.Errorf("guarded=%t,want%t: %s", got, tc.guarded, tc.sql)
		}
	}
}
