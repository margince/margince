// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Choices use admitted current work before it is folded into groups.
// A pin admits a choice without turning the contact's silence into an obligation.
func (s *Service) relationshipChoices(ctx context.Context, day crmcontracts.Attention, work []ranked, scope string) (
	[]ranked, []crmcontracts.WorklistItem, []crmcontracts.WorklistSourceUnavailable, scopeNote,
) {
	choices := relationshipCandidates(day)
	choices, refused := admitBySource(ctx, choices)
	choices, note := s.narrowToScope(ctx, choices, scope, s.taskOwner)
	combined := dropDecayAlreadyWaiting(slices.Concat(work, choices))
	choices = nil
	for _, row := range combined {
		if row.item.Source == sourceDecay {
			choices = append(choices, row)
		}
	}
	choices = dropDone(applyPins(choices, s.pinned), s.marked, day.AsOf)
	optional := make([]ranked, 0, len(choices))
	for _, row := range choices {
		if row.pinned {
			work = append(work, row)
		} else {
			optional = append(optional, row)
		}
	}
	return work, renderInOrder(stampAsOf(optional, day.AsOf), readerOf(ctx)), refused, note
}

func dropDecayAlreadyWaiting(rows []ranked) []ranked {
	held := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceDecay {
			continue
		}
		if row.item.PromiseRecipients != nil {
			for _, recipient := range *row.item.PromiseRecipients {
				held[recipient.ContactId.String()] = true
			}
		}
		if !row.contact.IsZero() {
			held[row.contact.String()] = true
		}
		if row.item.Subject != nil && row.item.Subject.Type == subjectContact {
			held[row.item.Subject.Id.String()] = true
		}
	}
	return slices.DeleteFunc(rows, func(row ranked) bool {
		return row.item.Source == sourceDecay && held[row.item.Id]
	})
}

func relationshipCandidates(day crmcontracts.Attention) []ranked {
	return appendLane(nil, sourceDecay, day.RelationshipDecay, day.AsOf, classifyDecay)
}

func (s *Service) relationshipsChosen() bool {
	for ref, pinned := range s.pinned {
		if pinned && ref.Source == string(sourceDecay) {
			return true
		}
	}
	return false
}
