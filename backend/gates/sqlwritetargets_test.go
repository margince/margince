// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

// SQL assignments stay attached to their own target, including unresolved
// fragments and mutation branches inside compound statements.

package gates

import (
	"slices"
	"testing"
)

func TestTheWriteReaderKeepsEachStatementsOwnTargets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, sql       string
		tables, columns []string
	}{
		{"quoted", `UPDATE "deal" SET "amount_minor"=@amount`, []string{"deal"}, []string{"amount_minor"}},
		{"qualified", `UPDATE "public" . "deal" AS d SET d.currency=@currency`, []string{"deal"}, []string{"currency"}},
		{"two inserts", `INSERT INTO other(name) VALUES (@name); INSERT INTO deal(currency) VALUES (@currency)`, []string{"other", "deal"}, []string{"name", "currency"}},
		{"two updates", `UPDATE other SET name=@name WHERE id=@id; UPDATE deal SET currency=@currency WHERE id=@id`, []string{"other", "deal"}, []string{"name", "currency"}},
		{"row assignment", `UPDATE deal SET (amount_minor,currency)=(@amount,@currency)`, []string{"deal"}, []string{"amount_minor", "currency"}},
		{"nested where", `UPDATE deal SET name=(SELECT name FROM other WHERE id=@id), currency=@currency WHERE id=@id`, []string{"deal"}, []string{"name", "currency"}},
		{"case before protected column", `UPDATE deal SET name=CASE WHEN active THEN @a ELSE @b END, currency=@currency WHERE id=@id`, []string{"deal"}, []string{"name", "currency"}},
		{"keyword column", `UPDATE deal SET "where"=@value, currency=@currency`, []string{"deal"}, []string{"where", "currency"}},
		{"incomplete insert", `INSERT INTO deal (`, []string{"deal"}, nil},
		{"value cannot write", `SELECT 'UPDATE deal SET currency=@currency'`, nil, nil},
		{"comment cannot write", `SELECT 1 /* UPDATE deal SET currency=@currency */`, nil, nil},
		{"merge", `MERGE INTO deal d USING source s ON d.id=s.id WHEN MATCHED THEN UPDATE SET currency=s.currency`, []string{"deal"}, []string{"currency"}},
		{"routine body", `CREATE FUNCTION f() RETURNS void AS $$ BEGIN UPDATE deal SET currency=@currency; END $$ LANGUAGE plpgsql`, []string{"deal"}, []string{"currency"}},
		{"escaped value", `SELECT E'escaped\' UPDATE fake SET currency=1'; UPDATE deal SET currency=@currency`, []string{"deal"}, []string{"currency"}},
		{"merge insert without columns", `MERGE INTO deal d USING source s ON d.id=s.id WHEN MATCHED THEN UPDATE SET name=s.name WHEN NOT MATCHED THEN INSERT VALUES(s.id)`, []string{"deal", "deal"}, []string{"name"}},
		{"merge delete", `MERGE INTO deal_product p USING source s ON p.deal_id=s.id WHEN MATCHED THEN DELETE`, []string{"deal_product"}, nil},
		{"locking read", `SELECT id FROM deal FOR NO KEY UPDATE`, nil, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var tables, columns []string
			for _, write := range sqlWrites(test.sql) {
				tables = append(tables, write.table)
				columns = append(columns, write.cols...)
			}
			if !slices.Equal(tables, test.tables) || !slices.Equal(columns, test.columns) {
				t.Fatalf("tables=%v columns=%v; expected %v %v", tables, columns, test.tables, test.columns)
			}
		})
	}
}

func TestRawSQLCannotHideAnyProductionLineOwnedColumn(t *testing.T) {
	t.Parallel()
	files := parsePackageFiles(t, lineOwner)
	columns, err := stringSliceVar(files, "lineOwnedColumns", stringConstants(files))
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		statement := `UPDATE "public"."deal" SET "` + column + `"=@value WHERE id=@id`
		writes := sqlWrites(statement)
		if len(writes) != 1 || writes[0].table != "deal" || !lineMoneyColumns(writes[0].cols, columns) {
			t.Fatalf("line-owned column escaped: %s", statement)
		}
	}
	if lineMoneyColumns([]string{"name"}, columns) {
		t.Fatal("an unrelated field was classified as line-owned money")
	}
	if !lineMoneyColumns(nil, columns) {
		t.Fatal("an unresolved assignment was classified as unrelated")
	}
}

func TestLineSQLCensusRefusesPlantedAssignmentAndConstructorBypasses(t *testing.T) {
	t.Parallel()
	files := parsePackageFiles(t, lineOwner)
	constants := stringConstants(files)
	columns, err := stringSliceVar(files, "lineOwnedColumns", constants)
	if err != nil {
		t.Fatal(err)
	}
	columns = append(columns, constants["commercialModeColumn"], constants["commercialRevisionColumn"])
	for _, statement := range []string{
		"UPDATE deal SET name=@name, ?=@amount WHERE id=@id",
		"UPDATE deal SET name=@name, ? WHERE id=@id",
		"WITH x AS (SELECT 1) UPDATE ? SET amount_minor=@amount",
		"MERGE INTO deal d USING source s ON d.id=s.id WHEN NOT MATCHED THEN INSERT VALUES(s.id)",
		"UPDATE deal SET commercial_mode=@mode",
		"UPDATE deal SET commercial_revision=@revision",
	} {
		if len(lineSQLProblems(statement, constants["dealTable"], columns, lineSQLSite{dir: lineOwner})) == 0 {
			t.Errorf("writer escaped: %s", statement)
		}
	}
	if problems := lineSQLProblems("UPDATE deal SET name=@name", constants["dealTable"], columns, lineSQLSite{dir: lineOwner}); len(problems) > 0 {
		t.Fatalf("unrelated write refused: %v", problems)
	}
}

func TestTheDealConstructorCannotIntroduceLineModeOrAnUpsert(t *testing.T) {
	t.Parallel()
	constants := stringConstants(parsePackageFiles(t, lineOwner))
	site := lineSQLSite{dir: lineOwner, function: "Store.createDealInTx", modeColumns: []string{constants["commercialModeColumn"], constants["commercialRevisionColumn"]}}
	for _, statement := range []string{
		"INSERT INTO deal(name," + constants["commercialModeColumn"] + ") VALUES(@name,@mode)",
		"INSERT INTO deal(name) VALUES(@name) ON\n CONFLICT(id) DO UPDATE SET name=@name",
	} {
		if problems := lineSQLProblems(statement, constants["dealTable"], nil, site); len(problems) == 0 {
			t.Errorf("constructor bypass escaped: %s", statement)
		}
	}
	if problems := lineSQLProblems("INSERT INTO deal(name) VALUES(@name)", constants["dealTable"], nil, site); len(problems) > 0 {
		t.Fatalf("estimate constructor refused: %v", problems)
	}
}
