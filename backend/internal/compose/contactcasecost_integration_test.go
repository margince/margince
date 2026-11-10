// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactCaseCensusCostDoesNotGrowWithItsCases(t *testing.T) {
	c := setupCases(t)
	first := seedDistinctContactCase(t, c, 0)
	sent := &sentStatements{}
	config := c.e.Pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := contacts.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](c.e.WS)))
	lane := attentionContactCases{store: store}
	read := func(want int) int {
		t.Helper()
		sent.sql = nil
		rows, err := lane.OpenCases(c.rep(c.e.Rep1), attention.CaseQuery{Scope: attention.CaseScopeMine, Recipient: c.e.Rep1, Limit: 200})
		if err != nil || len(rows) != want {
			t.Fatalf("read %d cases, want %d: %v", len(rows), want, err)
		}
		count := 0
		for _, sql := range sent.sql {
			if !strings.HasPrefix(sql, "SELECT set_config") && !strings.EqualFold(sql, "begin") && !strings.EqualFold(sql, "commit") {
				count++
			}
		}
		return count
	}
	small := read(1)
	for i := range 199 {
		seedDistinctContactCase(t, c, i+1)
	}
	large := read(200)
	if small != large || large > 10 {
		t.Fatalf("one case cost %d statements, two hundred cost %d; budget 10", small, large)
	}
	page, _, err := store.ListContactCases(c.rep(c.e.Rep1), contacts.ContactCaseListInput{Scope: "mine", Recipient: c.e.Rep1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	single := c.read(c.rep(c.e.Rep1), t, first[0])
	for _, view := range page {
		if view.AnchorId == single.AnchorId && !reflect.DeepEqual(view, single) {
			t.Fatalf("batch and single case disagree: %+v / %+v", view, single)
		}
	}
}

func seedDistinctContactCase(t *testing.T, c *caseEnv, n int) []ids.UUID {
	t.Helper()
	name := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(n))))
	phone := fmt.Sprintf("+4930555%04d", 1000+n)
	owner := ids.From[ids.UserKind](c.e.Rep1)
	var members []ids.UUID
	for range 2 {
		contact, err := c.store.CreateContact(c.e.Admin(), contacts.CreateContactInput{
			FullName: name, Source: "manual", OwnerID: &owner,
			Phones: []contacts.ContactPhoneInput{{Phone: phone, PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, ids.UUID(contact.Id))
	}
	return members
}
