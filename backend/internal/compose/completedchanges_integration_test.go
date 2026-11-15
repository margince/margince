// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHomeCompletedChangesRemainDiscoverableAcrossHistoryPages(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	for i := range 63 {
		actor := fmt.Sprintf("system:completed-fixture-%d", i)
		if i >= 60 {
			actor = "system:grouped-fixture"
		}
		_, err := e.Contacts.CreateContact(principal.SystemActing(e.Admin(), actor), contacts.CreateContactInput{FullName: fmt.Sprintf("Automatic contact %d", i), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
	}
	svc := NewApprovalsServiceWithEffects(e.Pool)
	home, err := newMagicService(e.Pool, svc, func() time.Time { return now }).Read(e.Admin(), nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Done) != 3 {
		t.Fatalf("preview items=%d, want three", len(home.Done))
	}
	history := newAttentionService(e.Pool, svc, func() time.Time { return now })
	seen := map[ids.UUID]bool{}
	pages, groups := 0, 0
	var cursor *string
	for {
		page, err := history.HandledForYou(e.Admin(), cursor)
		if err != nil {
			t.Fatal(err)
		}
		if page.Since == nil || !page.Since.Equal(home.Since) {
			t.Fatalf("window differs: Home=%v history=%v", home.Since, page.Since)
		}
		for _, receipt := range page.Receipts {
			if seen[ids.UUID(receipt.Id)] {
				t.Fatal("history repeated a receipt")
			}
			seen[ids.UUID(receipt.Id)] = true
			if receipt.CompletedChange == nil {
				t.Fatal("history omitted the common completed change")
			}
			if receipt.CompletedChange.Count != nil && *receipt.CompletedChange.Count == 3 {
				groups++
			}
			if receipt.CompletedChange.ReceiptIds != nil {
				for _, id := range *receipt.CompletedChange.ReceiptIds {
					seen[ids.UUID(id)] = true
				}
			}
		}
		pages++
		if page.NextCursor == nil {
			break
		}
		if pages == 1 {
			if _, err := e.Contacts.CreateContact(principal.SystemActing(e.Admin(), "system:grouped-fixture"), contacts.CreateContactInput{FullName: "Arrived after the history pin", Source: "manual"}); err != nil {
				t.Fatal(err)
			}
		}
		cursor = page.NextCursor
		if pages > 3 {
			t.Fatal("history did not finish")
		}
	}
	if pages != 2 || groups != 1 {
		t.Fatalf("pages=%d grouped changes=%d, want two pages and one group", pages, groups)
	}
	for _, line := range home.Done {
		if !seen[ids.UUID(line.Id)] {
			t.Fatalf("preview %s disappeared from history", line.Id)
		}
		if line.ReceiptIds != nil {
			for _, id := range *line.ReceiptIds {
				if !seen[ids.UUID(id)] {
					t.Fatalf("preview receipt %s disappeared", id)
				}
			}
		}
	}
	withheld := e.As(e.Rep1, nil, principal.Permissions{})
	hidden, err := history.HandledForYou(withheld, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Receipts) != 0 {
		t.Fatal("history disclosed unreadable automatic contacts")
	}
	if hidden.SourcesUnavailable == nil || len(*hidden.SourcesUnavailable) == 0 {
		t.Fatal("unreadable sources were reported as a quiet day")
	}
}
