// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extingress converts what an extension unit hands the ingress port
// into the core's own shapes, and holds the checks that belong to the
// conversion. It writes nothing itself: every durable effect is capture's.
package extingress

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// UserUUID reads a user id a unit named, such as the colleague of a task. An id that is not a
// canonical UUID cannot reach here: Record.Validate refuses it.
func UserUUID(id extension.UserID) ids.UUID {
	parsed, err := ids.Parse(string(id))
	if err != nil {
		return ids.UUID{}
	}
	return parsed
}

// ParticipantsOf maps the published roster onto the core's.
//
// Record.Validate has refused an over-cap roster and a party with no identity
// before this runs. CapParticipants here applies the core's own bound where the
// core can see it. The refusal a unit reads comes from the door.
func ParticipantsOf(parties []extension.Participant) []connector.MessageParticipant {
	if len(parties) == 0 {
		return nil
	}
	mapped := make([]connector.MessageParticipant, 0, len(parties))
	for _, p := range parties {
		mapped = append(mapped, connector.MessageParticipant{
			Email:         p.Email,
			ChannelUserID: p.Account,
			DisplayName:   p.Name,
			Role:          p.Role,
		})
	}
	return connector.CapParticipants(mapped).Participants
}

// CounterpartyOf maps the published counterparty onto the core's and stamps what
// the SOURCE declared onto it.
//
// The declaration is stamped and never taken from the record, because a unit
// that stated its own trust could widen it per record. It is reduced to the one
// question the core asks: may this address corroborate. The key list belongs to
// the manifest an operator reads.
func CounterpartyOf(cp extension.Counterparty, declared extension.IngressSource) connector.Counterparty {
	return connector.Counterparty{
		Email:       cp.Email,
		DisplayName: cp.DisplayName,
		Domain:      cp.Domain,
		Direction:   cp.Direction,
		// The binding that makes the record repliable. The caller holds it to the
		// unit's declared transports. A unit that bound an identity under a core
		// connector's provider would write into the table the reply path reads
		// its recipients from.
		//
		// DisplayName lands on Username, a display-only handle that nothing
		// routes, authorizes or deduplicates on.
		ChannelIdentity: connector.ChannelIdentity{
			Provider:      cp.ChannelIdentity.Provider,
			ChannelUserID: cp.ChannelIdentity.ChannelUserID,
			Username:      cp.ChannelIdentity.DisplayName,
		},
	}.WithDeclaredEmailMerge(DeclaresMergeKey(declared, extension.MergeKeyEmail))
}

// DeclaresMergeKey reports whether a source vouched for one identity key.
func DeclaresMergeKey(declared extension.IngressSource, key extension.MergeKey) bool {
	return slices.Contains(declared.Merges, key)
}

// RefuseUndeclaredMergeKey is the unit-facing half of the merge-key gate: a
// source may offer an address to MATCH on only if it vouched for one. Capture's
// own admitCounterpartyKeys holds the invariant for every caller of Upsert.
//
// It asks only about an address that corroborates a human named by a channel
// identity. The address of a mail-shaped record is that record's identity. It
// belongs to no declaration and is left alone.
func RefuseUndeclaredMergeKey(declared extension.IngressSource, cp extension.Counterparty) error {
	namedByChannel := cp.ChannelIdentity.Provider != "" && cp.ChannelIdentity.ChannelUserID != ""
	if !namedByChannel || cp.Email == "" {
		return nil
	}
	if DeclaresMergeKey(declared, extension.MergeKeyEmail) {
		return nil
	}
	return fmt.Errorf("%w: ingress source %q carries a counterparty address to match on but declares no %q merge key",
		extension.ErrInvalid, declared.System, string(extension.MergeKeyEmail))
}

// PartsOf maps a record's files onto the core's and derives again what a unit
// must not certify. The type comes from the bytes, and the name is made safe.
// A claimed type that the bytes contradict is kept as DeclaredType.
func PartsOf(files []extension.InboundFile) []connector.Part {
	if len(files) == 0 {
		return nil
	}
	parts := make([]connector.Part, 0, len(files))
	for _, f := range files {
		sniffed := extension.SniffContentType(f.Body)
		declared := f.DeclaredType
		if declared == "" && f.ContentType != "" && f.ContentType != sniffed {
			declared = f.ContentType
		}
		parts = append(parts, connector.Part{
			Ordinal:      f.Ordinal,
			Filename:     extension.SafeFilename(f.Filename, f.Ordinal),
			ContentType:  sniffed,
			DeclaredType: declared,
			Body:         f.Body,
		})
	}
	return parts
}
