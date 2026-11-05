// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type certCommitmentParty struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func prepareCommitmentParties(input []certCommitmentParty, identities map[string]ids.UUID) ([]commitmentParty, map[string]string, error) {
	var verified activities.MessageParties
	references := map[string]string{}
	for _, party := range input {
		if party.Key == "" {
			return nil, nil, errors.New("a participant has no fixture key")
		}
		identityKey := party.Kind + ":" + party.Key
		id, found := identities[identityKey]
		if !found {
			id = ids.NewV7()
			identities[identityKey] = id
		}
		wireID := openapi_types.UUID(id)
		wire := crmcontracts.EmailParty{Address: party.Address, DisplayName: &party.Name}
		switch party.Kind {
		case "seat":
			wire.UserId = &wireID
		case string(recordTypeContact):
			wire.ContactId = &wireID
		default:
			return nil, nil, fmt.Errorf("participant %q has unknown kind %q", party.Key, party.Kind)
		}
		reference := party.Kind + ":" + id.String()
		if old, exists := references[party.Key]; exists && old != reference {
			return nil, nil, fmt.Errorf("participant key %q names different identities", party.Key)
		}
		references[party.Key] = reference
		switch party.Role {
		case commitmentSenderRole:
			verified.From = append(verified.From, wire)
		case "to":
			verified.To = append(verified.To, wire)
		case "cc":
			verified.Cc = append(verified.Cc, wire)
		case "organizer":
			verified.Organizers = append(verified.Organizers, wire)
		case "attendee":
			verified.Attendees = append(verified.Attendees, wire)
		default:
			return nil, nil, fmt.Errorf("participant %q has unsupported role %q", party.Key, party.Role)
		}
	}
	return verifiedCommitmentParties(verified), references, nil
}

func certPromiserKeys(references ...map[string]string) map[string]string {
	keys := map[string]string{}
	for _, group := range references {
		for key, ref := range group {
			keys[ref] = key
		}
	}
	return keys
}

func validateExpectedRecipients(expected []string, references map[string]string) error {
	seen := map[string]bool{}
	for _, key := range expected {
		if !strings.HasPrefix(references[key], "contact:") || seen[key] {
			return fmt.Errorf("expected recipient %q must name a distinct verified contact", key)
		}
		seen[key] = true
	}
	return nil
}

func expectedRecipientsMatch(expected, refs []string, keys map[string]string) bool {
	if expected == nil {
		return true
	}
	actual := make([]string, 0, len(refs))
	for _, ref := range refs {
		key, found := keys[ref]
		if !found {
			return false
		}
		actual = append(actual, key)
	}
	wanted := slices.Clone(expected)
	slices.Sort(wanted)
	slices.Sort(actual)
	return slices.Equal(wanted, actual)
}
