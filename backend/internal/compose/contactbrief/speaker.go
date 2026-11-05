// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import (
	"strconv"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Speaker is who wrote an activity, as a model reads it: a fixed kind and a
// separate name. A sender types their own display name, so a name that claims
// to be a colleague lands in Name and never changes Kind.
type Speaker struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// The speaker kinds, in the voice the brief is written in. "them" and "you"
// are exported for the floors that name who wrote last.
const (
	SpeakerThem       = "them"
	SpeakerYou        = "you"
	speakerColleague  = "colleague"
	speakerContact    = "contact"
	speakerMachine    = "machine"
	speakerUnresolved = "unresolved"
)

// maxQuotedSenderName caps a sender-typed name that reaches the model.
const maxQuotedSenderName = 64

// SpeakerFor names who wrote an activity, from the server's author projection.
//
// "them" only when the page's contact is the author; "you" only when the
// author is the reader. Anybody else is named with their kind. No author, no
// speaker. Every model input folds its rows through this one function.
func SpeakerFor(voice *crmcontracts.ActivityVoice) *Speaker {
	if voice == nil || voice.Author == nil {
		return nil
	}
	author := voice.Author
	// A sending system stays one even when a contact was minted for its address.
	if author.Kind == crmcontracts.ActivityAuthorKindMachine {
		return &Speaker{Kind: speakerMachine, Name: senderName(author)}
	}
	if voice.ContactRole != nil && *voice.ContactRole == crmcontracts.ActivityVoiceContactRoleAuthor {
		return &Speaker{Kind: SpeakerThem}
	}
	if author.IsReader {
		return &Speaker{Kind: SpeakerYou}
	}
	switch author.Kind {
	case crmcontracts.ActivityAuthorKindColleague:
		return &Speaker{Kind: speakerColleague, Name: recordName(author)}
	case crmcontracts.ActivityAuthorKindContactRecord:
		return &Speaker{Kind: speakerContact, Name: recordName(author)}
	default:
		return &Speaker{Kind: speakerUnresolved, Name: senderName(author)}
	}
}

// recordName is a colleague's or contact's name as our own records hold it.
func recordName(author *crmcontracts.ActivityAuthor) string {
	if author.DisplayName != nil && strings.TrimSpace(*author.DisplayName) != "" {
		return strings.TrimSpace(*author.DisplayName)
	}
	return senderName(author)
}

// senderName names an author no record vouches for: the address, else the
// typed name, quoted and capped so it reads as data.
func senderName(author *crmcontracts.ActivityAuthor) string {
	if author.Address != nil && strings.TrimSpace(*author.Address) != "" {
		return strings.TrimSpace(*author.Address)
	}
	if author.DisplayName == nil {
		return ""
	}
	name := []rune(strings.TrimSpace(*author.DisplayName))
	if len(name) > maxQuotedSenderName {
		name = name[:maxQuotedSenderName]
	}
	if len(name) == 0 {
		return ""
	}
	return strconv.Quote(string(name))
}
