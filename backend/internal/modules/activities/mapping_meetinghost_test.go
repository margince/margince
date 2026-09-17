// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// host_user_id pairs with kind meeting and nothing else, the same pairing
// meeting_status keeps and for the same reason: the column's CHECK constrains
// the kind, so a mail carrying a host is a write that fails at the database
// with an error naming a constraint rather than the field the caller sent.
//
// The forward case matters because a rep's week is counted off this column. A
// mapping that dropped it would leave the minute-taker credited with somebody
// else's meeting — silently, and in the host's own view of their week.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLogActivityInputCarriesAMeetingsHost(t *testing.T) {
	host := openapi_types.UUID(ids.NewV7())

	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:       crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		HostUserId: &host,
		Source:     "human",
	})
	if err != nil {
		t.Fatalf("mapping a minuted meeting: %v", err)
	}
	if in.HostUserID == nil || in.HostUserID.UUID != ids.UUID(host) {
		t.Fatalf("HostUserID = %v, want %v — dropped, a minuted meeting counts into the minute-taker's week", in.HostUserID, host)
	}
}

func TestLogActivityInputRefusesAHostOnAMail(t *testing.T) {
	host := openapi_types.UUID(ids.NewV7())

	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:       crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail,
		HostUserId: &host,
		Source:     "human",
	})

	var fault *KindFieldError
	if !errors.As(err, &fault) {
		t.Fatalf("err = %v, want a KindFieldError — a mail carrying a host must be a 422 naming the field, not a constraint violation", err)
	}
	if field, _, _ := fault.FieldFault(); field != "host_user_id" {
		t.Fatalf("FieldFault names %q, want host_user_id", field)
	}
}

// Omission is how a caller says "I held it", so it must reach the store as
// nothing at all — the fallback is what turns that into the acting human, and
// a mapping that invented a zero id here would write a host nobody is.
func TestLogActivityInputLeavesAnUnnamedHostToTheStore(t *testing.T) {
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:   crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		Source: "human",
	})
	if err != nil {
		t.Fatalf("mapping a meeting with no host named: %v", err)
	}
	if in.HostUserID != nil {
		t.Fatalf("an unnamed host arrived as %v, want nil so the store fills in the caller", in.HostUserID)
	}
}
