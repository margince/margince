// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The conversation claims behind the commitments card and the what-matters
// card (ADR-0097 D1).
//
// One read, both cards. They render different kinds of the same rows, so
// reading them twice would be two queries that could disagree about what this
// person said.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// claimsSection reads what was promised, asked and decided.
//
// The gate is the ACTIVITY read, not a gate of its own: every claim quotes a
// captured message, and a reader who may not open the message may not read the
// quote. The store's own query carries that predicate.
//
// A project scope narrows it like the timeline: a claim is evidence from a
// conversation, so one made on another engagement's mail is not this
// engagement's commitment.
func (s *Service) claimsSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	claims, err := s.people.ClaimsForPerson(ctx, tx, personID, opts.ProjectID, sectionCap)
	if err != nil {
		return err
	}
	owed, err := s.openCommitments(ctx, tx, personID, opts)
	if err != nil {
		return err
	}
	out.Claims = ptr(withOpenCommitments(claims, owed))
	return nil
}

// withOpenCommitments returns the display page with any open promise it did
// not already carry appended.
//
// The section shows the newest claims of every kind, capped at sectionCap, and
// the moment ladder reads that same list. On a record with more than a page of
// recent claims the two purposes disagree: a promise made in June and due
// tomorrow is nowhere on the newest page, so the card reports nothing owed
// while the record owes something this week.
//
// Appended rather than merged into the order, because the section's order is
// its own — newest first is what a reader scrolling the card expects — and the
// ladder does not read position. It ranks over the whole set through
// kernel/owedwork.
func withOpenCommitments(page, owed []crmcontracts.ConversationClaim) []crmcontracts.ConversationClaim {
	shown := make(map[openapi_types.UUID]bool, len(page))
	for _, claim := range page {
		shown[claim.Id] = true
	}
	for _, claim := range owed {
		if !shown[claim.Id] {
			page = append(page, claim)
		}
	}
	return page
}

// openCommitments reads the open promises of OURS on this record — a different
// set from the claims section above, and read separately for that reason.
//
// Held by: TestAnOpenPromiseOffTheNewestPageStillReachesTheLadder
// (claims_commitments_test.go), which fails when a promise the display page
// cannot carry stops reaching the ladder.
//
// The section shows the NEWEST claims of every kind, capped. The moment ladder
// asks which promise is most urgent, and off that page it cannot tell: a
// commitment made in June and due tomorrow sits behind twenty-five newer
// decisions and questions, so the card would report no promise at all on a
// record that owes one this week.
//
// A project scope narrows it the way the section is narrowed. A promise made
// in another engagement's mail is not this engagement's to show.
func (s *Service) openCommitments(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions,
) ([]crmcontracts.ConversationClaim, error) {
	if err := requireRead(ctx, "activity"); err != nil {
		return nil, err
	}
	return s.people.OpenCommitmentsForPerson(ctx, tx, personID, opts.ProjectID, commitmentScanCap)
}

// commitmentScanCap bounds the promise read behind the moment card. Wider than
// the display cap because the card ranks over the set rather than showing it:
// a bound that decided which promise is most urgent would be the read stopping
// early wearing the ladder's clothes.
const commitmentScanCap = 200
