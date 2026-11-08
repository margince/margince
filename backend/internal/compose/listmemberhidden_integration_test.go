// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A caller may change a Shortlist and not read its record type. They are told
// the record is not there, and the gate notes that it hid the record.
func TestAShortlistMemberOfAnUnreadableTypeIsAHiddenRow(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["list"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	lists := collections.NewStore(e.DB())
	list, err := lists.CreateList(e.As(e.AdminUser, nil, perms), collections.CreateListInput{Name: "Unreadable shortlist", EntityType: "deal"})
	if err != nil {
		t.Fatal(err)
	}

	blind := maps.Clone(perms.Objects)
	blind["deal"] = principal.ObjectGrant{}
	perms.Objects = blind
	ctx, recorded := auth.RecordChecksForTest(e.As(e.AdminUser, nil, perms))
	_, err = lists.AddMember(ctx, list.ID, collections.MemberChange{EntityType: "deal", EntityID: ids.NewV7(), Reason: collections.ReasonChosen})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("adding a deal without deal.read: %v, want not found", err)
	}
	if got := recorded().Refused; !slices.Contains(got, "row:deal") {
		t.Errorf("the refusal noted %v, want row:deal", got)
	}
}
