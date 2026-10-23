// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Rooms is the room's own gated reads, the ones the tab beside the brief makes.
type Rooms interface {
	GetRoom(ctx context.Context, id ids.DealRoomID) (crmcontracts.DealRoom, error)
	ListParticipants(ctx context.Context, roomID ids.DealRoomID, activeOnly bool) ([]crmcontracts.DealRoomParticipant, error)
	ListDocuments(ctx context.Context, roomID ids.DealRoomID) ([]crmcontracts.DealRoomDocument, error)
	ListThreads(ctx context.Context, roomID ids.DealRoomID, documentID *ids.UUID) ([]crmcontracts.DealRoomThread, error)
}

// Service writes the brief.
type Service struct {
	pool  *pgxpool.Pool
	rooms Rooms
	lane  Completer
	now   func() time.Time
}

// NewService binds the brief to the room's reads, with no model lane: the
// deterministic floor answers until WithLane binds one.
func NewService(pool *pgxpool.Pool, rooms Rooms, now func() time.Time) *Service {
	return &Service{pool: pool, rooms: rooms, now: now}
}

// WithLane binds the summarize lane that writes the brief.
func (s *Service) WithLane(lane Completer) *Service {
	s.lane = lane
	return s
}

// Get writes the brief for one room.
func (s *Service) Get(ctx context.Context, roomID ids.DealRoomID) (crmcontracts.DealRoomBrief, error) {
	// A reading aid for a human reader; an agent reading through a passport has
	// the records themselves.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.DealRoomBrief{}, err
	}
	in, err := s.assemble(ctx, roomID)
	if err != nil {
		return crmcontracts.DealRoomBrief{}, err
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	sentences, by := Write(ai.WithSubject(ctx, roomID.Ref(), in.Title), s.lane, in, lang)
	return crmcontracts.DealRoomBrief{
		RoomId:      openapi_types.UUID(roomID.UUID),
		GeneratedAt: s.now().UTC(),
		GeneratedBy: by,
		Sentences:   wireSentences(sentences),
	}, nil
}

// assemble reads the room first: a room the caller cannot see refuses before
// anything about it is read.
func (s *Service) assemble(ctx context.Context, roomID ids.DealRoomID) (Input, error) {
	room, err := s.rooms.GetRoom(ctx, roomID)
	if err != nil {
		return Input{}, err
	}
	participants, err := s.rooms.ListParticipants(ctx, roomID, true)
	if err != nil {
		return Input{}, fmt.Errorf("room brief: reading who may enter: %w", err)
	}
	documents, err := s.rooms.ListDocuments(ctx, roomID)
	if err != nil {
		return Input{}, fmt.Errorf("room brief: reading the documents: %w", err)
	}
	threads, err := s.rooms.ListThreads(ctx, roomID, nil)
	if err != nil {
		return Input{}, fmt.Errorf("room brief: reading the conversation: %w", err)
	}
	return FromRoom(room, participants, documents, threads), nil
}

// wireSentences drops the whole sentence when any citation is not an id,
// rather than showing a claim on partial evidence.
func wireSentences(in []Sentence) []crmcontracts.DealRoomBriefSentence {
	out := make([]crmcontracts.DealRoomBriefSentence, 0, len(in))
	for _, sentence := range in {
		evidence := make([]crmcontracts.DealRoomBriefEvidence, 0, len(sentence.Evidence))
		for _, cited := range sentence.Evidence {
			parsed, err := ids.Parse(cited.EntityID)
			if err != nil {
				evidence = nil
				break
			}
			evidence = append(evidence, crmcontracts.DealRoomBriefEvidence{
				EntityId:   openapi_types.UUID(parsed),
				EntityType: crmcontracts.DealRoomBriefEvidenceEntityType(cited.EntityType),
			})
		}
		if len(evidence) == 0 {
			continue
		}
		out = append(out, crmcontracts.DealRoomBriefSentence{
			Text:     sentence.Text,
			Nature:   crmcontracts.DealRoomBriefSentenceNature(sentence.Nature),
			Evidence: evidence,
		})
	}
	return out
}
