// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extingress

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
)

// IsChatMessage reports whether a record files through the shared chat
// function. That is a message whose counterparty is named by an account on the
// transport the message travelled on. Every other activity keeps the general
// mapping.
func IsChatMessage(rec extension.Record) bool {
	return rec.Kind() == extension.KindActivity &&
		rec.Activity.Kind == extension.ActivityKindMessage &&
		rec.Activity.DurationSeconds == nil &&
		rec.Counterparty.ChannelIdentity.ChannelUserID != "" &&
		rec.Counterparty.ChannelIdentity.Provider == rec.Activity.ChannelProvider
}

// ChatSource is what the core knows about the connection a record came in on.
type ChatSource struct {
	// CapturedBy is the connector identity the core stamped for the unit, and
	// SourceSystem the provenance it derived for the declared source.
	CapturedBy   string
	SourceSystem string
	// Member is the member whose credential produced the record.
	Member ids.UUID
	Source extension.IngressSource
	// Now dates a record whose original time the unit does not know.
	Now time.Time
}

// ChatInput maps a unit's chat record onto the binding and message the shared
// chat function files.
//
// A source that pinned a posture is member-owned: the message is that member's
// correspondence and is born under the pinned posture. Any other source files
// as the company's, as before.
func ChatInput(src ChatSource, rec extension.Record) (connector.ChatBinding, connector.ChatMessage, error) {
	binding := connector.ChatBinding{
		Provider:      rec.Activity.ChannelProvider,
		CapturedBy:    src.CapturedBy,
		SourceSystem:  src.SourceSystem,
		Ownership:     connector.ChatOwnedByWorkspace,
		PinnedPosture: string(src.Source.PinnedPosture),
	}
	if src.Source.PinnedPosture != "" {
		binding.Ownership = connector.ChatOwnedByMember
		binding.OwnerUserID = src.Member
	}
	direction := rec.Activity.Direction
	if direction == "" {
		direction = rec.Counterparty.Direction
	}
	cp := rec.Counterparty
	occurredAt := rec.Activity.OccurredAt
	precision := rec.Precision()
	if occurredAt.IsZero() && precision == extension.PrecisionUnknown {
		occurredAt = src.Now
	}
	chosen, err := ChosenRef(rec.ChosenSubject)
	if err != nil {
		return binding, connector.ChatMessage{}, err
	}
	return binding, connector.ChatMessage{
		Direction: direction,
		Counterparty: connector.ChatParty{
			UserID:        cp.ChannelIdentity.ChannelUserID,
			DisplayName:   cp.DisplayName,
			Username:      cp.ChannelIdentity.DisplayName,
			Email:         cp.Email,
			Domain:        cp.Domain,
			EmailMayMatch: DeclaresMergeKey(src.Source, extension.MergeKeyEmail),
		},
		Body:        rec.Activity.Body,
		OccurredAt:  occurredAt,
		OccurredDay: precision != extension.PrecisionInstant,
		Raw:         rec.Raw,
		Keys: &connector.ChatOwnKeys{
			SourceID:              rec.Key,
			ThreadKey:             rec.ThreadKey,
			SourceIDNamesAContact: rec.KeyNamesAContact,
		},
		Subject:         rec.Activity.Subject,
		ProviderPrivate: rec.Activity.ProviderPrivate,
		Addresses:       rec.Addresses,
		Participants:    ParticipantsOf(rec.Participants),
		ChosenSubject:   chosen,
		Parts:           PartsOf(rec.Parts),
		PartDrops:       rec.PartDrops,
	}, nil
}

// ChosenRef maps a chosen subject onto the core's entity reference, or nil.
func ChosenRef(subject *extension.ChosenSubject) (ref *datasource.EntityRef, err error) {
	if subject == nil {
		return ref, err
	}
	id, err := ids.Parse(subject.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: the chosen subject is not a row id", apperrors.ErrInvalidArgument)
	}
	switch subject.Kind {
	case extension.SubjectContact:
		return &datasource.EntityRef{Type: datasource.EntityContact, ID: id}, nil
	case extension.SubjectLead:
		return &datasource.EntityRef{Type: datasource.EntityLead, ID: id}, nil
	}
	return nil, fmt.Errorf("%w: the chosen subject is neither a contact nor a lead", apperrors.ErrInvalidArgument)
}
