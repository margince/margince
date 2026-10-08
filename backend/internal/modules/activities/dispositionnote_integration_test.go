// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A note carries no disposition: it has no direction and no conversation, so
// setting or clearing one is refused as not_a_message.
func TestMarkingANoteAsNotSalesIsRefusedAsNotAMessage(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	body := "Called the buyer back"
	note, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "note", Body: &body, Source: "test", OccurredAt: &requestInstant,
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}},
	})
	if err != nil {
		t.Fatalf("log the note: %v", err)
	}

	id := ids.From[ids.ActivityKind](ids.UUID(note.Id))
	for name, act := range map[string]func() error{
		"setting not sales":          func() error { return store.SetThreadNotSales(e.asSeat(e.rep), id) },
		"clearing it (scope=thread)": func() error { return store.ClearThreadNotSales(e.asSeat(e.rep), id) },
	} {
		var refused *values.ParseError
		if err := act(); !errors.As(err, &refused) || refused.Code != "not_a_message" {
			t.Errorf("%s on a note answered %v, want the not_a_message refusal", name, err)
		}
	}
}
