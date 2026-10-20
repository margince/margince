// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestRelinkPreservesDistinctChatAccountsInTheSameRole(t *testing.T) {
	e, ctx, people, accounts, id := setupChannelParticipantRelink(t)
	in := activities.RelinkActivityInput{EntityType: "contact", EntityID: people[1], ReplaceExistingOfType: true}
	preview, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id.UUID}, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 1 {
		t.Fatalf("preview: %+v", preview)
	}
	if len(preview[0].Participants) != 3 {
		t.Fatalf("participant effects: %+v", preview[0].Participants)
	}
	moved := map[string]bool{}
	for _, participant := range preview[0].Participants {
		if participant.ChannelUserID == nil && (participant.Role != "from" || !participant.Move || participant.ContactID != people[0]) {
			t.Fatalf("sender effect: %+v", participant)
		}
		if participant.ChannelUserID != nil {
			moved[*participant.ChannelUserID] = participant.Move
			if participant.ContactID != people[0] {
				t.Fatalf("unexpected source: %+v", participant)
			}
		}
	}
	if !maps.Equal(moved, map[string]bool{"acct-a": true, "acct-b": true}) {
		t.Fatalf("account effects: %+v", moved)
	}
	if _, err := e.Activities.RelinkActivity(ctx, id, in); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND channel_user_id IS NOT NULL`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(accounts) {
		t.Fatalf("participant count = %d, want %d", count, len(accounts))
	}
	for account := range accounts {
		_, contact, _, found := rosterRow(t, id, account)
		if !found || contact == nil || *contact != people[1] {
			t.Fatalf("account %s lost or not relinked: %v", account, contact)
		}
	}
}

func setupChannelParticipantRelink(t *testing.T) (*Env, context.Context, []ids.UUID, map[string]ids.UUID, ids.ActivityID) {
	t.Helper()
	e := Setup(t)
	perms := AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, perms)
	var people []ids.UUID
	for _, name := range []string{"Original contact", "Correct contact"} {
		person, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		people = append(people, ids.UUID(person.Id))
	}
	accounts := map[string]ids.UUID{"acct-a": people[0], "acct-b": people[0], "acct-c": people[1]}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for account, person := range accounts {
			bound, err := contacts.ResolveOrCreateChannelIdentity(ctx, tx, ids.From[ids.ContactKind](person), connector.ChannelIdentity{Provider: rosterProvider, ChannelUserID: account})
			if err != nil {
				return err
			}
			if bound.UUID != person {
				return fmt.Errorf("account %s bound to %s", account, bound)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: activities.KindMessage, ChannelProvider: rosterProvider, Subject: new("Support group"), Direction: new("inbound"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: people[0]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
	stampRoster(t, e, id, false,
		connector.MessageParticipant{ChannelUserID: "acct-a", Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-b", Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-c", Role: connector.ParticipantRoleAttendee})
	return e, ctx, people, accounts, id
}

func TestContactMergeParticipantPreviewMatchesDistinctChatAccounts(t *testing.T) {
	e, ctx, people, accounts, id := setupChannelParticipantRelink(t)
	shared, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: activities.KindMessage, ChannelProvider: rosterProvider, Subject: new("Shared conversation"), Direction: new("inbound"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: people[0]}, {EntityType: "contact", EntityID: people[1]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return e.Contacts.SetChannelIdentityBlocked(ctx, tx, connector.ChannelIdentity{Provider: rosterProvider, ChannelUserID: "acct-a"}, true, "demo-bot", 1)
	}); err != nil {
		t.Fatal(err)
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", people[0], people[1], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Participants) != 4 {
		t.Fatalf("participants: %+v", preview.Participants)
	}
	if len(preview.ChannelIdentities) != 2 {
		t.Fatalf("channel bindings: %+v", preview.ChannelIdentities)
	}
	for _, identity := range preview.ChannelIdentities {
		if identity.ID.IsZero() || identity.Version < 1 || identity.Provider != rosterProvider || identity.Username != nil || (identity.Account != "acct-a" && identity.Account != "acct-b") || identity.Blocked != (identity.Account == "acct-a") {
			t.Fatalf("binding effect: %+v", identity)
		}
	}
	moved := map[string]bool{}
	for _, participant := range preview.Participants {
		if participant.ActivityID == ids.UUID(shared.Id) {
			if !participant.Collapses {
				t.Fatalf("duplicate seat: %+v", participant)
			}
			continue
		}
		if participant.ActivityID != id.UUID || participant.Collapses {
			t.Fatalf("effect: %+v", participant)
		}
		if participant.ChannelUserID != nil {
			moved[*participant.ChannelUserID] = true
		}
	}
	if !maps.Equal(moved, map[string]bool{"acct-a": true, "acct-b": true}) {
		t.Fatalf("accounts: %v", moved)
	}
	if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](people[0]), ids.From[ids.ContactKind](people[1]), &preview.SourceVersion); err != nil {
		t.Fatal(err)
	}
	reachable := map[string]bool{}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		identities, err := contacts.ReachableChannelIdentities(ctx, tx, ids.From[ids.ContactKind](people[1]), rosterProvider)
		if err != nil {
			return err
		}
		for _, identity := range identities {
			reachable[identity.ChannelUserID] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(reachable, map[string]bool{"acct-b": true, "acct-c": true}) {
		t.Fatalf("merged reachability: %v", reachable)
	}
	var survivorSeats int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_participant WHERE activity_id = @activity AND contact_id = @target`,
		pgx.StrictNamedArgs{"activity": ids.UUID(shared.Id), "target": people[1]}).Scan(&survivorSeats); err != nil {
		t.Fatal(err)
	}
	if survivorSeats != 1 {
		t.Fatalf("shared conversation has %d survivor seats", survivorSeats)
	}
	var retiredSeats int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_participant WHERE contact_id = @source`, pgx.StrictNamedArgs{"source": people[0]}).Scan(&retiredSeats); err != nil {
		t.Fatal(err)
	}
	if retiredSeats != 0 {
		t.Fatalf("%d seats still name the retired contact", retiredSeats)
	}
	for account := range accounts {
		_, contact, _, found := rosterRow(t, id, account)
		if !found || contact == nil || *contact != people[1] {
			t.Fatalf("account %s: %v", account, contact)
		}
	}
}

func TestContactMergeParticipantPreviewRefusesAnUnreadableRoster(t *testing.T) {
	e, _, people, _, id := setupChannelParticipantRelink(t)
	perms := AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.Rep1, nil, perms)
	stampRoster(t, e, id, false, connector.MessageParticipant{ChannelUserID: "acct-a", Role: connector.ParticipantRoleBCC})
	if _, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", people[0], people[1], nil, nil); err != apperrors.ErrNotFound {
		t.Fatalf("hidden BCC roster: %v", err)
	}
}

func TestContactMergeParticipantPreviewRefusesATruncatedRoster(t *testing.T) {
	e, ctx, people, _, _ := setupChannelParticipantRelink(t)
	for i := range 498 {
		if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
			Kind: activities.KindMessage, ChannelProvider: rosterProvider, Subject: new(fmt.Sprintf("Conversation %d", i)), Direction: new("inbound"), Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: people[0]}},
		}); err != nil {
			t.Fatal(err)
		}
		if i == 496 {
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", people[0], people[1], nil, nil)
			if err != nil || len(preview.Participants) != 500 {
				t.Fatalf("complete roster: %d, %v", len(preview.Participants), err)
			}
		}
	}
	if _, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", people[0], people[1], nil, nil); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("oversized roster: %v", err)
	}
}

func TestContactMergeParticipantPreviewRefusesAnUnwiredReader(t *testing.T) {
	e, ctx, people, _, _ := setupChannelParticipantRelink(t)
	e.Contacts.WithParticipantRosterReader(nil)
	if _, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", people[0], people[1], nil, nil); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("unwired reader: %v", err)
	}
}

func TestContactMergeParticipantReadGrantDoesNotDependOnRosterPresence(t *testing.T) {
	e, ctx, people, _, _ := setupChannelParticipantRelink(t)
	empty, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "No conversations", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	perms := AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	perms.Objects["activity"] = principal.ObjectGrant{}
	restricted := e.As(e.Rep1, nil, perms)
	for _, source := range []ids.UUID{people[0], ids.UUID(empty.Id)} {
		if _, err := e.Contacts.PreviewMergeSurvivorship(restricted, "contact", source, people[1], nil, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("roster-dependent admission: %v", err)
		}
	}
}

func TestContactMergePreviewRefusesMaskedChannelBindings(t *testing.T) {
	e, _, people, _, _ := setupChannelParticipantRelink(t)
	perms := AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	perms.RoleKeys = []string{"rep"}
	perms.RowScope = principal.RowScopeOwn
	perms.FieldMasks = []principal.FieldMask{{Object: "contact", Field: "reachability", Condition: principal.MaskAlways}}
	preview, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, perms), "contact", people[0], people[1], nil, nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(preview.ChannelIdentities) != 0 {
		t.Fatalf("masked channel bindings: %+v, %v", preview.ChannelIdentities, err)
	}
}
