// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnInvoiceCursorFromAnEarlierOrderStartsANewList(t *testing.T) {
	legacy := struct {
		Connection ids.UUID  `json:"c"`
		Due        time.Time `json:"d"`
		Invoice    ids.UUID  `json:"i"`
	}{ids.NewV7(), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), ids.NewV7()}
	token, err := storekit.EncodeOpaque(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = paymentCursor(&token)
	refusal, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "cursor" || refusal.Fields[0].Code != "cursor_param_mismatch" {
		t.Fatalf("an old invoice position must request a restart instead of endless retries: %v", err)
	}
}

func TestAnIncompleteCurrentInvoiceCursorIsStillMalformed(t *testing.T) {
	token, err := storekit.EncodeOpaque(paymentPosition{Connection: ids.NewV7(), Due: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Invoice: ids.NewV7(), Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = paymentCursor(&token)
	if _, ok := errors.AsType[*storekit.MalformedCursorError](err); !ok {
		t.Fatalf("an incomplete position was accepted: %v", err)
	}
}
