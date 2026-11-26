// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noSenderStamp is the explicit no-op a fixture injects when it proves
// nothing about who sent a message.
func noSenderStamp(context.Context, pgx.Tx, []ids.UUID, ids.UUID) ([]ids.UUID, error) {
	return nil, nil
}

func TestALinkWithoutTheSenderRuleFailsClosed(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).stampProvenSenders(context.Background(), nil, nil, ids.New[ids.ContactKind]())
	if !errors.Is(err, errSenderStampUnwired) {
		t.Fatalf("an unwired store answered %v, want errSenderStampUnwired", err)
	}
	if _, err := NewStore(nil).WithSenderStamp(noSenderStamp).stampProvenSenders(context.Background(), nil, nil, ids.New[ids.ContactKind]()); err != nil {
		t.Fatalf("a store with an explicit no-op answered %v, want nil", err)
	}
}
