// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func sellerSeat(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:seller", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

// The audience question has no answer without a seat, so each seller read
// refuses before it reaches the database. The nil transaction proves it.
func TestASellerReadWithNoSeatBoundRefuses(t *testing.T) {
	ctx := context.Background()
	room := ids.New[ids.DealRoomKind]()
	if _, err := documentRows(ctx, nil, room); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the room's document list = %v, want permission denied", err)
	}
	if _, err := readDocument(ctx, nil, room, ids.New[ids.DealRoomDocumentKind]()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("one room document = %v, want permission denied", err)
	}
	if _, err := engagementByParticipant(ctx, nil, room); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the roster's downloaded titles = %v, want permission denied", err)
	}
}

func TestASeatWithoutActivityReadSeesNoCapturedFile(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ctx := sellerSeat(map[string]principal.ObjectGrant{"deal_room": {Read: true}})
	clause, err := sellerReadsTheCarrier(ctx, arg)
	if err != nil {
		t.Fatalf("sellerReadsTheCarrier: %v", err)
	}
	if clause != " AND a.entity_type <> 'activity'" || len(args) != 0 {
		t.Errorf("clause = %q with %d args, want every captured file withheld and nothing bound", clause, len(args))
	}
}

// A seat that reads activities is asked about the carrying message itself, and
// every placeholder the clause names is one it bound.
func TestASeatWithActivityReadIsAskedAboutTheCarryingMessage(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ctx := sellerSeat(map[string]principal.ObjectGrant{"deal_room": {Read: true}, "activity": {Read: true}})
	clause, err := sellerReadsTheCarrier(ctx, arg)
	if err != nil {
		t.Fatalf("sellerReadsTheCarrier: %v", err)
	}
	if !strings.Contains(clause, "carrier.id = a.entity_id") {
		t.Errorf("clause = %q, want it to read the message the file came with", clause)
	}
	if len(args) == 0 {
		t.Fatalf("clause bound no arguments, want the seat's audience test bound to it")
	}
	for n := 1; n <= len(args); n++ {
		if !strings.Contains(clause, fmt.Sprintf("$%d", n)) {
			t.Errorf("argument %d is bound but $%d appears nowhere in %q", n, n, clause)
		}
	}
	if strings.Contains(clause, fmt.Sprintf("$%d", len(args)+1)) {
		t.Errorf("clause names $%d, one past the %d arguments it bound", len(args)+1, len(args))
	}
}
