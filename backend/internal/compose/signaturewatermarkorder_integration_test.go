// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSignatureWatermarkOrdersEqualTimeMailAndRejectsAnOlderCompletion(t *testing.T) {
	h := setupHistory(t)
	first := h.writes(t, 0, signed("Director"))
	second := h.writes(t, 0, signed("Head of Procurement"))
	low, high := first, second
	if bytes.Compare(low[:], high[:]) > 0 {
		low, high = high, low
	}
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), h.e.WS), "agent:enrich")
	store := newContactsStore(InstallationDB(h.e.Pool))
	contact := ids.From[ids.ContactKind](h.contact)
	if err := store.MarkSignatureRead(ctx, contact, low, 3, contacts.SignatureRead{}, contacts.SignatureWork{}); err != nil {
		t.Fatal(err)
	}
	candidates, err := store.SignatureCandidatesForContacts(ctx, []ids.ContactID{contact}, 100, true)
	if err != nil || len(candidates) != 1 || candidates[0].ActivityID != high {
		t.Fatalf("equal-time candidates=%+v err=%v", candidates, err)
	}
	if err := store.MarkSignatureRead(ctx, contact, high, 3, contacts.SignatureRead{}, contacts.SignatureWork{}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSignatureRead(ctx, contact, low, 3, contacts.SignatureRead{}, contacts.SignatureWork{}); err != nil {
		t.Fatal(err)
	}
	candidates, err = store.SignatureCandidatesForContacts(ctx, []ids.ContactID{contact}, 100, true)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("watermark rewound: %+v err=%v", candidates, err)
	}
	if got := h.watermark(t); got != high.String()+":3" {
		t.Fatalf("watermark=%s want=%s:3", got, high)
	}
}
