// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func samePromiseBeforeAttributionChanged(c Commitment, offer CommitmentTaskProposal) bool {
	if c.Span == "" || offer.SourceActivityID != c.SourceActivityID {
		return false
	}
	matches := func(party string) bool {
		return offer.Locator == commitmentLocator(c.SourceActivityID, claimKindOurs, party, c.Span)
	}
	if matches("us") || matches("name:") || matches(unnamedPromiserKey("")) {
		return true
	}
	if c.Seat == nil && c.Theirs == nil {
		seat := offer.PromiserSeatID
		if seat == nil {
			seat = offer.SeatID
		}
		if seat != nil && matches("seat:"+seat.String()) {
			return true
		}
		for _, party := range c.parties {
			if party.Seat != nil && matches(party.Ref) {
				return true
			}
		}
		return false
	}
	if !matches(unnamedPromiserKey(offer.Party)) {
		return false
	}
	return sameReportedPromiser(offer.Party, c)
}

func sameReportedPromiser(reported string, c Commitment) bool {
	reported = strings.TrimSpace(reported)
	if reported == "" || strings.EqualFold(reported, strings.TrimSpace(c.Party)) {
		return true
	}
	var matched string
	for _, party := range c.parties {
		words := strings.Fields(party.Name)
		names := strings.EqualFold(reported, strings.TrimSpace(party.Name)) || strings.EqualFold(reported, party.Address)
		if len(words) > 0 {
			names = names || strings.EqualFold(reported, words[0])
		}
		if !names {
			continue
		}
		if matched != "" && matched != party.Ref {
			return false
		}
		matched = party.Ref
	}
	if c.Seat != nil {
		return matched == "seat:"+c.Seat.String()
	}
	if c.Theirs != nil {
		return matched == "contact:"+c.Theirs.String()
	}
	return false
}

func (d *CommitmentDispatcher) resolvedWorkBefore(ctx context.Context, tx pgx.Tx, c Commitment) (bool, error) {
	if c.Span == "" {
		return false, nil
	}
	var locators []string
	for _, party := range c.parties {
		if party.Seat != nil {
			locators = append(locators, commitmentLocator(c.SourceActivityID, claimKindOurs, party.Ref, c.Span))
		}
	}
	if len(locators) > 0 {
		written, err := d.tasks.CommitmentTaskWritten(ctx, tx, locators...)
		if err != nil || written {
			return written, err
		}
	}
	history, err := commitmentTaskHistoryTx(ctx, tx, c.SourceActivityID)
	if err != nil {
		return false, err
	}
	for _, task := range history {
		if task.Assignee != nil && task.Locator == commitmentLocator(c.SourceActivityID, claimKindOurs, "seat:"+task.Assignee.String(), c.Span) {
			return true, nil
		}
	}
	claims, err := d.claims.CustomerPromiseHistoryTx(ctx, tx, c.SourceActivityID)
	if err != nil {
		return false, err
	}
	for _, claim := range claims {
		if claim.Locator == commitmentLocator(c.SourceActivityID, claimKindTheirs, "contact:"+claim.Contact.String(), c.Span) {
			return true, nil
		}
	}
	return false, nil
}
