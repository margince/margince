// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

const commitmentSenderRole = "from"

type commitmentParty struct {
	Ref, Role, Name, Address string
	Seat, Contact            *ids.UUID
}

func verifiedCommitmentParties(parties activities.MessageParties) []commitmentParty {
	var out []commitmentParty
	for _, header := range []struct {
		role    string
		parties []crmcontracts.EmailParty
	}{
		{commitmentSenderRole, parties.From},
		{"to", parties.To},
		{"cc", parties.Cc},
		{"organizer", parties.Organizers},
		{"attendee", parties.Attendees},
	} {
		for _, party := range header.parties {
			candidate := commitmentParty{Role: header.role, Address: party.Address}
			if party.DisplayName != nil {
				candidate.Name = *party.DisplayName
			}
			switch {
			case party.UserId != nil:
				seat := ids.UUID(*party.UserId)
				candidate.Seat, candidate.Ref = &seat, "seat:"+seat.String()
			case party.ContactId != nil:
				contact := ids.UUID(*party.ContactId)
				candidate.Contact, candidate.Ref = &contact, "contact:"+contact.String()
			}
			out = append(out, candidate)
		}
	}
	return out
}

// commitmentPartyPrompt lists the participants under the short refs a reply
// cites them by; their record keys never reach the model.
func commitmentPartyPrompt(parties []commitmentParty, source string, refs *promptref.Refs) string {
	var out strings.Builder
	for _, party := range parties {
		address := ""
		if quotedCommitmentAddress(source, party.Address) {
			address = party.Address
		}
		ref := ""
		if party.Ref != "" {
			ref = refs.For(party.Ref)
		}
		fmt.Fprintf(&out, "%q %q %q %q\n", ref, party.Role, party.Name, address)
	}
	return out.String()
}

// commitmentPartyRefs names each verified participant p1, p2, … in order; a
// participant with no record has no ref to cite.
func commitmentPartyRefs(parties []commitmentParty) *promptref.Refs {
	refs := promptref.New("p")
	for _, party := range parties {
		if party.Ref != "" {
			refs.For(party.Ref)
		}
	}
	return refs
}

// partyKey is the participant key a reply's ref names. A string that is no ref
// of this prompt stays as written, for the validator to refuse.
func partyKey(refs *promptref.Refs, ref string) string {
	if key, ok := refs.ID(ref); ok {
		return key
	}
	return ref
}

// partyKeys reads each ref of a list back to its participant key, keeping nil
// as nil: an omitted list and an empty one mean different things.
func partyKeys(refs *promptref.Refs, cited []string) []string {
	if cited == nil {
		return nil
	}
	keys := make([]string, len(cited))
	for i, ref := range cited {
		keys[i] = partyKey(refs, ref)
	}
	return keys
}

func referencedPromiser(ref string, parties []commitmentParty) (commitmentParty, bool) {
	if ref == "" {
		return commitmentParty{}, false
	}
	for _, party := range parties {
		if party.Ref == ref {
			return party, true
		}
	}
	return commitmentParty{}, false
}

func soleCommitmentCounterparty(parties []commitmentParty) (ids.UUID, bool) {
	seen := map[ids.UUID]bool{}
	var contacts []ids.UUID
	for _, party := range parties {
		if party.Role == "cc" || party.Contact == nil || seen[*party.Contact] {
			continue
		}
		seen[*party.Contact] = true
		contacts = append(contacts, *party.Contact)
	}
	return onlyOne(contacts)
}

func (x *SignalExtractor) readCommitmentParties(ctx context.Context, thread *settledThread) error {
	if len(thread.Messages) == 0 {
		return nil
	}
	reader := extractorContext(ctx, signalScanActor)
	return database.WithWorkspaceTx(reader, x.pool, func(tx pgx.Tx) error {
		for i := range thread.Messages {
			parties, err := x.messages.PartiesOf(reader, tx, thread.Messages[i].ID)
			if err != nil {
				return err
			}
			thread.Messages[i].Parties = verifiedCommitmentParties(parties)
		}
		return nil
	})
}

func (p *TranscriptProposer) readCommitmentParties(ctx context.Context, activityID ids.ActivityID) ([]commitmentParty, error) {
	reader := extractorContext(ctx, transcriptProposalActor)
	var parties []commitmentParty
	err := database.WithWorkspaceTx(reader, p.pool, func(tx pgx.Tx) error {
		found, err := p.messages.PartiesOf(reader, tx, activityID.UUID)
		if err != nil {
			return err
		}
		parties = verifiedCommitmentParties(found)
		return nil
	})
	return parties, err
}

func attributeCommitment(c *Commitment, ref string, parties []commitmentParty) (string, string) {
	c.parties = parties
	promiser, found := referencedPromiser(ref, parties)
	if found && ambiguousCommitmentPromiser(promiser, parties, c.Quote) {
		c.Party, found = promiser.Name, false
	}
	if !found {
		reported := strings.TrimSpace(c.Party)
		if reported != "" {
			c.Party = reported
			return claimKindOurs, unnamedPromiserKey(reported)
		}
		c.Party = ""
		return claimKindOurs, unnamedPromiserKey("")
	}
	c.Party = promiser.Name
	if c.Party == "" {
		c.Party = promiser.Address
	}
	if promiser.Contact != nil {
		contact := ids.From[ids.ContactKind](*promiser.Contact)
		c.Theirs = &contact
		return claimKindTheirs, promiser.Ref
	}
	c.Seat = promiser.Seat
	if recipient, one := soleCommitmentCounterparty(parties); one {
		contact := ids.From[ids.ContactKind](recipient)
		c.PromisedTo = &contact
		if !slices.Contains(linkedContacts(c.Links), recipient) {
			c.Links = append(c.Links, activities.ActivityLinkInput{EntityType: string(recordTypeContact), EntityID: recipient})
		}
	}
	return claimKindOurs, promiser.Ref
}

func ambiguousCommitmentPromiser(selected commitmentParty, parties []commitmentParty, quote string) bool {
	name := strings.TrimSpace(selected.Name)
	if name == "" {
		return false
	}
	addressCited := quotedCommitmentAddress(quote, selected.Address)
	for _, other := range parties {
		if other.Ref == selected.Ref || !strings.EqualFold(strings.TrimSpace(other.Name), name) {
			continue
		}
		if !addressCited || strings.EqualFold(other.Address, selected.Address) {
			return true
		}
	}
	return false
}

func commitmentPartyHasRole(parties []commitmentParty, ref, role string) bool {
	for _, party := range parties {
		if party.Ref == ref && party.Role == role {
			return true
		}
	}
	return false
}

func quotedCommitmentAddress(source, address string) bool {
	if address == "" {
		return false
	}
	pattern := `(?i)(^|[^\pL\pN._%+@-])` + regexp.QuoteMeta(address) + `($|[^\pL\pN.@-])`
	return regexp.MustCompile(pattern).MatchString(source)
}

func unnamedPromiserKey(reported string) string {
	name := strings.TrimSpace(reported)
	if name == "" {
		return "unresolved"
	}
	return "name:" + strings.ToLower(name)
}
