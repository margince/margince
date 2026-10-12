// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// A null in the body clears the field it names.
//
// The store already told absent from null through a double pointer: the outer
// one says the caller named the field, the inner carries the value. The mapping
// never produced the null case, so a cleared field arrived as a nil request
// pointer and read as "leave it alone".

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestANullClearsADealRoomField(t *testing.T) {
	t.Parallel()
	in := updateInput(crmcontracts.UpdateDealRoomRequest{},
		[]string{"welcome_message", "steward_user_id"}, nil)

	if in.WelcomeMessage == nil {
		t.Error("welcome_message was not named, so the null left the stored value in place")
	} else if *in.WelcomeMessage != nil {
		t.Errorf("welcome_message carries %v, want a nil inner pointer", *in.WelcomeMessage)
	}
	if in.StewardUserID == nil {
		t.Error("steward_user_id was not named, so the null left the stored value in place")
	} else if *in.StewardUserID != nil {
		t.Errorf("steward_user_id carries %v, want a nil inner pointer", *in.StewardUserID)
	}
}

// An absent field is still left alone, which is the distinction the double
// pointer exists to carry.
func TestAnAbsentDealRoomFieldIsUntouched(t *testing.T) {
	t.Parallel()
	in := updateInput(crmcontracts.UpdateDealRoomRequest{}, nil, nil)

	if in.WelcomeMessage != nil {
		t.Error("an absent welcome_message was named for writing")
	}
	if in.StewardUserID != nil {
		t.Error("an absent steward_user_id was named for writing")
	}
}
