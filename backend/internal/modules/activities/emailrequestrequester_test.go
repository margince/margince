// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unreachableTx is a database that answers no statement.
type unreachableTx struct {
	pgx.Tx
	err error
}

func (tx unreachableTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, tx.err
}

func (tx unreachableTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return scriptedRow(func(...any) error { return tx.err })
}

func TestAContactTheHeadersDoNotNameIsNeverRecordedAsTheSender(t *testing.T) {
	for theirs, want := range map[string]string{"from": "cc", "to": "to"} {
		if got := unstatedRecipientRole(theirs); got != want {
			t.Errorf("unstatedRecipientRole(%q) = %q, want %q", theirs, got, want)
		}
	}
}

func TestOnlyAnEmailThatStatesItsHeadersHasAnEnvelope(t *testing.T) {
	email := string(crmcontracts.ActivityKindEmail)
	if got := suppliedEnvelope(LogActivityInput{Kind: KindMeeting, EmailFrom: "a@customer.test"}); got != nil {
		t.Errorf("a meeting has envelope %+v, want none", got)
	}
	if got := suppliedEnvelope(LogActivityInput{Kind: email}); got != nil {
		t.Errorf("an email stating no header has envelope %+v, want none", got)
	}
	got := suppliedEnvelope(LogActivityInput{Kind: email, EmailFrom: " Ann@Customer.test ", EmailCc: []string{"BEN@customer.test"}})
	if got == nil || got.From != "ann@customer.test" || len(got.Cc) != 1 || got.Cc[0] != "ben@customer.test" {
		t.Errorf("envelope = %+v, want the headers lowercased and trimmed as contact_email stores them", got)
	}
}

// A filing that cannot be read files no reminder, rather than one on no page.
func TestAReminderWhoseFilingCannotBeReadIsNotWritten(t *testing.T) {
	down := errors.New("connection reset")
	seat := principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + "rep", UserID: ids.NewV7()}
	source := crmcontracts.Activity{Id: openapi_types.UUID(ids.NewV7())}
	if _, err := emailRequestTask(context.Background(), unreachableTx{err: down}, source, seat, time.Now()); !errors.Is(err, down) {
		t.Fatalf("emailRequestTask = %v, want the failed filing read", err)
	}
}

// A logged email whose headers cannot be matched records no participant at
// all, rather than falling back to calling every linked contact the sender.
func TestAHeaderMatchThatFailsFailsTheLog(t *testing.T) {
	down := errors.New("connection reset")
	author := "Imported author"
	in := LogActivityInput{
		Kind: string(crmcontracts.ActivityKindEmail), EmailFrom: "ann@customer.test",
		Links:  []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: ids.NewV7()}},
		Author: storekit.SourceAuthorInput{AuthorName: &author},
	}
	if err := stampLoggedParticipants(context.Background(), unreachableTx{err: down}, ids.From[ids.ActivityKind](ids.NewV7()), in); !errors.Is(err, down) {
		t.Fatalf("stampLoggedParticipants = %v, want the failed header match", err)
	}
}
