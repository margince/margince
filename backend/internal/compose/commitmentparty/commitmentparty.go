// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package commitmentparty is the verified participants of one conversation as
// a model reads them. Each is named by a short ref, read back to its participant.
package commitmentparty

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// Party is one verified participant. Ref is its participant key.
type Party struct {
	Ref, Role, Name, Address string
	Seat, Contact            *ids.UUID
}

// Refs names each verified participant p1, p2, … in order; a participant with
// no record has no ref to cite.
func Refs(parties []Party) *promptref.Refs {
	refs := promptref.New("p")
	for _, party := range parties {
		if party.Ref != "" {
			refs.For(party.Ref)
		}
	}
	return refs
}

// Key is the participant key a reply's ref names. A string that is no ref of
// this prompt stays as written, for the validator to refuse.
func Key(refs *promptref.Refs, ref string) string {
	if key, ok := refs.ID(ref); ok {
		return key
	}
	return ref
}

// Keys reads each ref of a list back to its participant key. It keeps nil as
// nil, because an omitted list and an empty one mean different things.
func Keys(refs *promptref.Refs, cited []string) []string {
	if cited == nil {
		return nil
	}
	keys := make([]string, len(cited))
	for i, ref := range cited {
		keys[i] = Key(refs, ref)
	}
	return keys
}

// Referenced is the participant whose key is ref.
func Referenced(ref string, parties []Party) (Party, bool) {
	if ref == "" {
		return Party{}, false
	}
	for _, party := range parties {
		if party.Ref == ref {
			return party, true
		}
	}
	return Party{}, false
}

// Outside is the contact an outside participant's ref names, or false for a
// colleague, an unverified party or no match.
func Outside(refs *promptref.Refs, ref string, parties []Party) (ids.UUID, bool) {
	party, found := Referenced(Key(refs, ref), parties)
	if !found || party.Contact == nil || party.Seat != nil {
		return ids.UUID{}, false
	}
	return *party.Contact, true
}
