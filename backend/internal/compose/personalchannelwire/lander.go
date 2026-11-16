// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannelwire

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
)

// ConnectorID is the capture principal a member's pushed message lands under. The
// sink admits only a connector whose id is the record's CapturedBy.
const ConnectorID = "connector:" + personalchannels.Provider

// Authority binds, for member, the connector principal capture admits, carrying
// that member's live grants. compose supplies it, because the live grants are
// identity's to read.
type Authority func(ctx context.Context, member ids.UUID) (context.Context, error)

// Lander lands one member's message through the one capture sink.
type Lander struct {
	Sink      *capture.Sink
	DB        *database.DB
	Authority Authority
}

var _ personalchannels.Lander = Lander{}

// Land lands m with its files, as the member, and says what became of it.
func (l Lander) Land(ctx context.Context, member ids.UUID, m personalchannels.NormalizedMessage, files []extension.InboundFile) (personalchannels.LandResult, error) {
	runCtx, err := l.Authority(ctx, member)
	if err != nil {
		return personalchannels.LandResult{}, err
	}
	key := connector.NaturalKey{SourceSystem: personalchannels.Provider, SourceID: m.SourceID}
	if existing, ok, err := l.landed(runCtx, key); err != nil || ok {
		return personalchannels.LandResult{Status: personalchannels.StatusDuplicate, ActivityID: existing}, err
	}
	rec, err := record(m, key, files)
	if err != nil {
		return personalchannels.LandResult{}, err
	}
	ref, err := l.Sink.Upsert(runCtx, rec)
	switch {
	case errors.Is(err, connector.ErrSkip):
		return personalchannels.LandResult{Status: personalchannels.StatusSkipped, Reason: "capture did not keep it"}, nil
	case err != nil:
		return personalchannels.LandResult{}, err
	}
	return personalchannels.LandResult{Status: personalchannels.StatusLanded, ActivityID: ref.ID}, nil
}

// landed reports whether key already names an activity, so a resend answers
// duplicate rather than landing.
func (l Lander) landed(ctx context.Context, key connector.NaturalKey) (ids.UUID, bool, error) {
	var id ids.UUID
	err := l.DB.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM activity WHERE source_system = $1 AND source_id = $2`,
			key.SourceSystem, key.SourceID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, false, nil
	}
	return id, err == nil, err
}

func record(m personalchannels.NormalizedMessage, key connector.NaturalKey, files []extension.InboundFile) (connector.NormalizedRecord, error) {
	raw, err := json.Marshal(m.PushMessage)
	if err != nil {
		return connector.NormalizedRecord{}, err
	}
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: key,
		Fields: capture.ActivityFields{
			Kind: "message", ChannelProvider: personalchannels.Provider,
			Subject: m.Subject, Body: m.Body, OccurredAt: m.OccurredAt, Direction: m.Direction,
		},
		Source:     personalchannels.Provider,
		CapturedBy: ConnectorID,
		ThreadKey:  m.ThreadKey,
		Raw:        raw,
		Counterparty: connector.Counterparty{
			DisplayName: m.Name, Direction: m.Direction,
			ChannelIdentity: connector.ChannelIdentity{Provider: personalchannels.Provider, ChannelUserID: m.Account},
		},
		Participants: participants(m),
		Parts:        files,
	}, nil
}

func participants(m personalchannels.NormalizedMessage) []connector.MessageParticipant {
	if len(m.Participants) == 0 {
		return nil
	}
	out := make([]connector.MessageParticipant, 0, len(m.Participants))
	for _, p := range m.Participants {
		out = append(out, connector.MessageParticipant{ChannelUserID: personalchannels.ParticipantIdentity(m, p.Account), DisplayName: p.Name})
	}
	return connector.CapParticipants(out).Participants
}
