// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MessengerContext combines fresh conversation authority with the local send state.
type MessengerContext struct {
	Unresolved   *MessengerAttempt
	Grant        MemberGrant
	Sender       messaging.Member
	Conversation messaging.Conversation
	AudienceKey  string
}

func (s *Store) admitMessengerRecord(ctx context.Context, kind string, id ids.UUID) error {
	if _, err := memberActor(ctx); err != nil {
		return err
	}
	if _, err := recordColumn(kind); err != nil {
		return err
	}
	if err := requireRecordRead(ctx, kind); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.readMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordRead(ctx, kind); err != nil {
			return err
		}
		return auth.EnsureVisibleExisting(ctx, tx, kind, id)
	})
}

// MessengerConversation rechecks native audience under the caller's personal grant.
func (s *Store) MessengerConversation(ctx context.Context, kind string, id ids.UUID, provider, conversation string) (MessengerContext, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerContext{}, err
	}
	grant, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return MessengerContext{}, err
	}
	room, err := client.Conversation(ctx, conversation)
	if err != nil {
		return MessengerContext{}, err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerContext{}, err
	}
	return s.messengerContext(ctx, kind, id, grant, room)
}

func (s *Store) messengerContext(ctx context.Context, kind string, id ids.UUID, grant MemberGrant, room messaging.Conversation) (MessengerContext, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerContext{}, err
	}
	var unresolved *MessengerAttempt
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.readMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		unresolved, err = unresolvedMessengerTx(ctx, tx, grant, room.ID)
		return err
	}); err != nil {
		return MessengerContext{}, err
	}
	key, err := messaging.AudienceKey(room)
	if err != nil {
		return MessengerContext{}, err
	}
	return MessengerContext{Unresolved: unresolved, Grant: grant, Sender: messaging.Member{ID: grant.Member, Name: grant.Name, Active: true}, Conversation: room, AudienceKey: key}, nil
}

// MessengerMembers finds recipients within the record's permitted Messenger context.
func (s *Store) MessengerMembers(ctx context.Context, kind string, id ids.UUID, provider, query, cursor string) ([]messaging.Member, string, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, "", err
	}
	_, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return nil, "", err
	}
	people, next, err := client.People(ctx, query, cursor)
	if err != nil {
		return nil, "", err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, "", err
	}
	return people, next, nil
}

// MessengerChannels discovers rooms after checking access to the current record.
func (s *Store) MessengerChannels(ctx context.Context, kind string, id ids.UUID, provider, cursor string) ([]messaging.Conversation, string, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, "", err
	}
	_, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return nil, "", err
	}
	rooms, next, err := client.Channels(ctx, cursor)
	if err != nil {
		return nil, "", err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return nil, "", err
	}
	return rooms, next, nil
}

// MessengerMessages checks record authority before reading a personal conversation.
func (s *Store) MessengerMessages(ctx context.Context, kind string, id ids.UUID, provider, conversation, root, cursor string) (messaging.Page, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return messaging.Page{}, err
	}
	_, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return messaging.Page{}, err
	}
	page, err := client.Messages(ctx, conversation, root, cursor)
	if err != nil {
		return messaging.Page{}, err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return messaging.Page{}, err
	}
	return page, nil
}

func rosterKey(people []string) (string, error) {
	ordered := slices.Clone(people)
	slices.Sort(ordered)
	for i, id := range ordered {
		if id == "" || i > 0 && ordered[i-1] == id {
			return "", apperrors.ErrInvalidArgument
		}
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// MessengerOpenMembers opens only the recipient set the caller selected and verified.
func (s *Store) MessengerOpenMembers(ctx context.Context, kind string, id ids.UUID, provider string, people []string) (MessengerContext, error) {
	if _, err := memberMutationActor(ctx); err != nil {
		return MessengerContext{}, err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerContext{}, err
	}
	if len(people) == 0 {
		return MessengerContext{}, apperrors.ErrInvalidArgument
	}
	grant, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return MessengerContext{}, err
	}
	key, err := rosterKey(append(slices.Clone(people), grant.Member))
	if err != nil {
		return MessengerContext{}, err
	}
	var room messaging.Conversation
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordRead(ctx, kind); err != nil {
			return err
		}
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		if err := holdMemberConsentTx(ctx, tx, grant); err != nil {
			return err
		}
		args := pgx.StrictNamedArgs{grantKey: grant.ID, versionKey: grant.Version, kindKey: kind, recordKey: id, "roster": key}
		lockKey := grant.ID.String() + ":" + kind + ":" + id.String() + ":" + key
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(@key,0))", pgx.StrictNamedArgs{keyKey: lockKey}); err != nil {
			return err
		}
		existing, conversation, err := boundMemberRoom(ctx, tx, client, args, key)
		if err != nil {
			return err
		}
		if existing.ID != "" {
			room = existing
			return nil
		}
		room, err = openMatchingPeople(ctx, client, people, key)
		if err != nil {
			return err
		}
		return rememberMemberRoomTx(ctx, tx, args, grant, room, conversation)
	})
	if err != nil {
		return MessengerContext{}, err
	}
	return s.messengerContext(ctx, kind, id, grant, room)
}

func matchesRoster(room messaging.Conversation, key string) bool {
	if !room.AudienceKnown || room.External {
		return false
	}
	people := make([]string, 0, len(room.Members))
	for _, member := range room.Members {
		if !member.Active || member.External || member.Bot {
			return false
		}
		people = append(people, member.ID)
	}
	current, err := rosterKey(people)
	return err == nil && current == key
}

func boundMemberRoom(ctx context.Context, tx pgx.Tx, client messaging.Client, args pgx.StrictNamedArgs, key string) (messaging.Conversation, string, error) {
	var conversation string
	err := tx.QueryRow(ctx, `SELECT conversation_id FROM chat_member_room WHERE grant_id=@grant AND grant_version=@version
 AND record_type=@kind AND record_id=@record AND roster_hash=@roster`, args).Scan(&conversation)
	if err == pgx.ErrNoRows {
		return messaging.Conversation{}, "", nil
	}
	if err != nil {
		return messaging.Conversation{}, "", err
	}
	existing, err := client.Conversation(ctx, conversation)
	if err != nil {
		return messaging.Conversation{}, conversation, err
	}
	if matchesRoster(existing, key) {
		return existing, conversation, nil
	}
	return messaging.Conversation{}, conversation, nil
}

func rememberMemberRoomTx(ctx context.Context, tx pgx.Tx, args pgx.StrictNamedArgs, grant MemberGrant, room messaging.Conversation, conversation string) error {
	args["conversation"] = room.ID
	var rowID ids.UUID
	err := tx.QueryRow(ctx, `INSERT INTO chat_member_room(grant_id,grant_version,record_type,record_id,roster_hash,conversation_id)
   VALUES(@grant,@version,@kind,@record,@roster,@conversation)
   ON CONFLICT(grant_id,grant_version,record_type,record_id,roster_hash) DO UPDATE SET conversation_id=EXCLUDED.conversation_id
   RETURNING id`, args).Scan(&rowID)
	if err != nil {
		return err
	}
	action := ModeCreate
	if conversation != "" {
		action = updateKey
	}
	var before map[string]any
	if conversation != "" {
		before = map[string]any{"grant_id": grant.ID, conversationIDKey: conversation}
	}
	_, err = storekit.Audit(ctx, tx, action, "chat_member_room", rowID, before, map[string]any{"grant_id": grant.ID, conversationIDKey: room.ID})
	return err
}

func openMatchingPeople(ctx context.Context, client messaging.Client, people []string, key string) (messaging.Conversation, error) {
	room, err := client.OpenPeople(ctx, people)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if !matchesRoster(room, key) {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	return room, nil
}
