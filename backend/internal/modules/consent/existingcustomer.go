// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The existing-customer exception, asked of the pack that grants it.
//
// UWG §7(3) lets a seller advertise SIMILAR goods to somebody who bought from
// them, on four conditions together. What shipped was a bare EXISTS over
// consent_existing_customer_flag: no jurisdiction, and one of the four actually
// enforced — the DDL's CHECK on optout_notice_given. The other three were
// free-text columns nothing compared against anything, under a comment claiming
// a live row "IS all four conditions".
//
// THE SIMILARITY CONDITION CANNOT BE ANSWERED BY THIS TREE TODAY, and that is
// why this file refuses rather than approximating. Two facts, either sufficient:
//
//   - Nothing on a send names the goods it advertises. The nearest field,
//     Request.MarketingPurpose, is a consent purpose key ("newsletter"), while
//     similar_goods_note is free text an operator typed about a sale ("espresso
//     machines"). Comparing them is not a weak check but a SATISFIABLE one: an
//     operator who types the purpose key into the note field would hold the
//     exception for that person forever, which is the hole this file exists to
//     close, relocated.
//   - The transmit phase carries no marketing purpose at all
//     (commsauthz.TransmitRequest has no such field), so a check resting on it
//     would allow at staging and deny at transmit — the disagreement
//     VerdictForPerson's own header says it exists to prevent.
//
// So an exception whose pack requires similarity is refused, with a reason that
// names WHY rather than reading as "no consent recorded". Nothing regresses: no
// production writer for consent_existing_customer_flag exists anywhere in this
// tree, so no installation is relying on the exception today. Making it work
// needs a structured goods class on the flag row and a server-derived goods
// field on the send — a schema change, not a comparison.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// existingCustomerAllows reports whether the exception the pack grants is
// satisfied for this person.
//
// A nil exception is a jurisdiction that grants none, which is the answer for
// every installation whose pack declares no MarketingExceptions and for one
// that names no country at all.
func existingCustomerAllows(ctx context.Context, tx pgx.Tx, personID string, exception *messaging.MarketingException) (bool, error) {
	if exception == nil {
		return false, nil
	}
	if exception.RequiresSimilarity {
		// Unanswerable, per the note above. Refusing is the direction that
		// costs a send rather than a complaint.
		return false, nil
	}
	var (
		saleReference string
		optoutNotice  bool
	)
	// One row per person: person_id is the primary key, so there is no history
	// to order and no newest to pick.
	err := tx.QueryRow(ctx, `
		SELECT sale_reference, optout_notice_given
		  FROM consent_existing_customer_flag
		 WHERE person_id = $1 AND revoked_at IS NULL`, personID).Scan(&saleReference, &optoutNotice)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the existing-customer flag: %w", err)
	}
	return conditionsMet(*exception, saleReference, optoutNotice), nil
}

// conditionsMet is the conditions this tree can answer, against one row.
//
// Split from the read so every condition is reachable by a test: the DDL will
// not store a row with optout_notice_given false, so the only way to ask what
// this code does with one is to hand it the values directly.
//
// RequiresNoObjection is not read here, and the reason is structural rather
// than an omission: VerdictForPerson blocks on a standing objection in its own
// first arm, for every class, before this is reached. There is no state where
// the flag matters.
func conditionsMet(exception messaging.MarketingException, saleReference string, optoutNotice bool) bool {
	// NOT NULL is not the same as recorded. The column accepts an empty
	// string, so a row written by a form that skipped its fields satisfies the
	// constraint and evidences nothing.
	if exception.RequiresSaleEvidence && strings.TrimSpace(saleReference) == "" {
		return false
	}
	// UNREACHABLE THROUGH THE SCHEMA, and kept anyway. The DDL's
	// consent_existing_customer_notice CHECK refuses a row with
	// optout_notice_given false, so no stored row can fail this — but the
	// condition is the PACK's to require, and reading the column is what makes
	// that requirement true rather than assumed. A merged exception from a
	// jurisdiction with no such CHECK would carry it too.
	if exception.RequiresCollectionTimeOptOut && !optoutNotice {
		return false
	}
	return true
}
