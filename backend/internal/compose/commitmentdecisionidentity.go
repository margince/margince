// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// takenUnderEarlierAttribution says whether a colleague took this promise over
// while no seat was named for it. The takeover's task is keyed on the unnamed
// form an earlier reading gave the promise. A later reading that names the
// promiser keys the promise apart, and must not write a second task beside the
// one the taker holds. The taken claim must be on the same message, the same
// recipient and the same quoted words. Two promises the earlier reading could
// not tell apart already shared one claim.
func (d *CommitmentDispatcher) takenUnderEarlierAttribution(ctx context.Context, tx pgx.Tx, c Commitment) (bool, error) {
	if c.Span == "" || c.Seat == nil || c.Theirs != nil {
		return false, nil
	}
	// The takeover holds this lock while it writes, so a takeover in flight
	// commits before this reads, or waits for this reading to commit.
	if err := lockPromiseSource(ctx, tx, c.SourceActivityID); err != nil {
		return false, err
	}
	var recipient *ids.UUID
	if c.PromisedTo != nil {
		recipient = &c.PromisedTo.UUID
	}
	var taken bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversation_claim c
		 WHERE c.source_activity_id = $1 AND c.kind = 'commitment_ours'
		   AND c.taken_by_user_id IS NOT NULL AND c.task_activity_id IS NOT NULL
		   AND c.evidence_fingerprint = ANY($2) AND c.source_quote = $3
		   AND ($4::uuid IS NULL OR c.contact_id = $4))`,
		c.SourceActivityID, earlierUnnamedLocators(c), c.Quote, recipient).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("compose: reading whether the promise was taken over: %w", err)
	}
	return taken, nil
}

// lockPromiseSource serializes a takeover and a reading of one message's
// promises, so neither writes a task the other cannot see.
func lockPromiseSource(ctx context.Context, tx pgx.Tx, source ids.UUID) error {
	return storekit.LockWriteIdentity(ctx, tx, "commitment_source", source.String())
}

// earlierUnnamedLocators are the locators the promise had while no seat was
// named. A reading then gave no name, or reported the seat's name or address
// without placing it.
func earlierUnnamedLocators(c Commitment) []string {
	keys := []string{"us", "name:", unnamedPromiserKey("")}
	names := []string{c.Party}
	for _, party := range c.parties {
		if party.Seat != nil && *party.Seat == *c.Seat {
			names = append(names, party.Name, party.Address)
		}
	}
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			keys = append(keys, unnamedPromiserKey(name))
		}
	}
	locators := make([]string, 0, len(keys))
	for _, key := range keys {
		locators = append(locators, commitmentLocator(c.SourceActivityID, claimKindOurs, key, c.Span))
	}
	return locators
}
