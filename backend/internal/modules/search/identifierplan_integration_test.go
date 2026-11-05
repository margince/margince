// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package search

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestIdentifierDiscoveryKeepsIndexedCompanyCandidates(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off; SET enable_indexscan = off"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"oreilly", "lena@exam"} {
		t.Run(query, func(t *testing.T) {
			caller := readerFor(entityCompany, entityContact, entityLead, "relationship")
			var args []any
			arg := func(value any) int { args = append(args, value); return len(args) }
			head, tail, fragment := bindTypedQuery(query, arg)
			identifier := bindIdentifier(query, arg)
			branches, err := admittedBranchSQL(caller, []string{entityCompany}, head, tail, fragment, identifier, true, arg)
			if err != nil {
				t.Fatal(err)
			}
			employer, err := employerArmSQL(caller, query, head, tail, fragment, identifier, arg)
			if err != nil {
				t.Fatal(err)
			}
			if employer == "" {
				t.Fatal("the employer arm was not admitted")
			}
			branches = append(branches, employer)
			statement := rankedShape{limit: 10}.statement(branches, "", arg)
			rows, err := conn.Query(ctx, "EXPLAIN "+statement, args...)
			if err != nil {
				t.Fatal(err)
			}
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				plan.WriteString(line + "\n")
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.String(), "idx_company_search") || strings.Contains(plan.String(), "Seq Scan on company ") {
				t.Fatalf("company discovery lost indexed candidates:\n%s", plan.String())
			}
		})
	}
}
