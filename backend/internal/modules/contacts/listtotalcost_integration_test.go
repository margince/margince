// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestExactListTotalUsesExhaustedFirstPage(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead_sorted", "lead_queue"} {
		for _, count := range []int{0, 1, 2, 3} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				e := setupCapturePrivacy(t)
				ctx := e.as(e.owner, principal.RowScopeAll)
				for i := range count {
					createTotalRecord(ctx, t, e.store, kind, i)
				}
				read := func(limit int, cursor *string) (int, storekit.Page, int64) {
					t.Helper()
					metered, meter := database.WithStatementMeter(ctx)
					size, page, err := readTotalRecords(metered, e.store, kind, limit, cursor)
					if err != nil {
						t.Fatal(err)
					}
					if page.Total == nil || *page.Total != count {
						t.Fatalf("total = %v, want %d", page.Total, count)
					}
					return size, page, meter.Cost().Statements
				}
				size, first, statements := read(2, nil)
				if size != min(count, 2) || first.HasMore != (count > 2) {
					t.Fatalf("size=%d has_more=%v, want size=%d has_more=%v", size, first.HasMore, min(count, 2), count > 2)
				}
				empty := ""
				emptySize, emptyPage, emptyStatements := read(2, &empty)
				if emptySize != size || emptyPage.HasMore != first.HasMore || emptyStatements != statements {
					t.Fatalf("empty cursor changed first page: size=%d has_more=%v statements=%d; want %d, %v, %d", emptySize, emptyPage.HasMore, emptyStatements, size, first.HasMore, statements)
				}
				if statements == 0 {
					t.Fatal("StatementMeter did not observe the list read")
				}
				if count == 2 {
					_, partial, countedStatements := read(1, nil)
					if !partial.HasMore || partial.NextCursor == "" {
						t.Fatal("one-row page must have a continuation")
					}
					if countedStatements != statements+1 {
						t.Fatalf("overflow used %d statements, exhausted page used %d; want exactly one extra count", countedStatements, statements)
					}
					tailSize, tail, _ := read(2, &partial.NextCursor)
					if tailSize != 1 || tail.HasMore || tail.NextCursor != "" {
						t.Fatalf("last page size=%d page=%+v, want one row and no continuation", tailSize, tail)
					}
				}
				if first.HasMore {
					tailSize, tail, _ := read(2, &first.NextCursor)
					if tailSize != 1 || tail.HasMore {
						t.Fatalf("last page size=%d has_more=%v, want one row and false", tailSize, tail.HasMore)
					}
				}
			})
		}
	}
}

func createTotalRecord(ctx context.Context, t *testing.T, store *Store, kind string, index int) {
	t.Helper()
	name := fmt.Sprintf("Total record %d", index)
	var err error
	switch kind {
	case "contact":
		_, err = store.CreateContact(ctx, CreateContactInput{FullName: name, Source: "manual"})
	case "company":
		_, err = store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	default:
		email := fmt.Sprintf("total%d@example.test", index)
		_, _, err = store.CreateLead(ctx, CreateLeadInput{FullName: &name, Email: &email, Source: "manual"})
	}
	if err != nil {
		t.Fatalf("create %s: %v", kind, err)
	}
}

func readTotalRecords(ctx context.Context, store *Store, kind string, limit int, cursor *string) (int, storekit.Page, error) {
	switch kind {
	case "contact":
		rows, page, err := store.ListContacts(ctx, ListContactsInput{Limit: &limit, Cursor: cursor})
		return len(rows), page, err
	case "company":
		rows, page, err := store.ListCompanies(ctx, ListCompaniesInput{Limit: &limit, Cursor: cursor})
		return len(rows), page, err
	default:
		var sort *string
		if kind == "lead_sorted" {
			order := "full_name"
			sort = &order
		}
		rows, page, err := store.ListLeads(ctx, ListLeadsInput{Limit: &limit, Cursor: cursor, Sort: sort})
		return len(rows), page, err
	}
}
