// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordPerson separates CRM assignment identity from native account matching.
type RecordPerson struct {
	ID     ids.UUID
	Kind   string
	Name   string
	Email  string
	Active bool
}

// TeamRecipient pairs a CRM assignee with native identity-verification state.
type TeamRecipient struct {
	Person RecordPerson
	Member string
	State  string
}

// MessengerRoster lets composition supply CRM assignment without sibling-module imports.
type MessengerRoster func(context.Context, pgx.Tx, string, ids.UUID, *ids.UUID) ([]RecordPerson, error)

// WithMessengerRoster shares CRM assignment with recipient comparison and group selection.
func (s *Store) WithMessengerRoster(roster MessengerRoster) *Store {
	s.messengerRoster = roster
	return s
}

// MessengerTeam resolves assigned people without automatically adding them to a room.
func (s *Store) MessengerTeam(ctx context.Context, kind string, id ids.UUID, provider string) ([]TeamRecipient, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, err
	}
	if s.messengerRoster == nil {
		return nil, messaging.ErrUnsupported
	}
	grant, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return nil, err
	}
	var people []RecordPerson
	linked := map[ids.UUID]string{}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		var facts recordFacts
		if err := readFacts(ctx, tx, kind, id, &facts); err != nil {
			return err
		}
		var err error
		people, err = s.messengerRoster(ctx, tx, kind, id, facts.owner)
		if err != nil {
			return err
		}
		linked, err = linkedTeamMembers(ctx, tx, people, provider, grant.Workspace)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]TeamRecipient, 0, len(people))
	for _, person := range people {
		recipient, err := resolveTeamRecipient(ctx, client, grant, person, linked[person.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, recipient)
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveTeamRecipient(ctx context.Context, client messaging.Client, grant MemberGrant, person RecordPerson, linked string) (TeamRecipient, error) {
	out := TeamRecipient{Person: person, State: "unresolved"}
	if !person.Active {
		out.State = "inactive"
		return out, nil
	}
	if person.Kind != userKey {
		return out, nil
	}
	if person.ID == grant.UserID {
		out.State = "self"
		out.Member = grant.Member
		return out, nil
	}
	if linked != "" {
		out.State = "matched"
		out.Member = linked
		return out, nil
	}
	if person.Email == "" {
		return out, nil
	}
	member, err := client.ResolvePerson(ctx, person.Email)
	if errors.Is(err, messaging.ErrUnavailable) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if member.ID == "" || !member.Active || member.External || member.Bot {
		return out, nil
	}
	out.State, out.Member = "matched", member.ID
	return out, nil
}

func linkedTeamMembers(ctx context.Context, tx pgx.Tx, people []RecordPerson, provider, workspace string) (map[ids.UUID]string, error) {
	linked := map[ids.UUID]string{}
	userIDs := make([]ids.UUID, 0, len(people))
	for _, person := range people {
		if person.Kind == userKind {
			userIDs = append(userIDs, person.ID)
		}
	}
	rows, err := tx.Query(ctx, "SELECT user_id,provider_user FROM chat_member_grant WHERE user_id=ANY(@users) AND provider=@provider AND provider_workspace=@workspace AND status='connected'", pgx.StrictNamedArgs{"users": userIDs, providerKey: provider, "workspace": workspace})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user ids.UUID
		var member string
		if err := rows.Scan(&user, &member); err != nil {
			return nil, err
		}
		linked[user] = member
	}
	return linked, rows.Err()
}
