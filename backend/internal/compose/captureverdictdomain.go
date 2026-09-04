// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The standing, workspace-wide half of a noise verdict.
//
// Hiding a message is about that message. Refusing a domain is about every
// colleague's future mail from it, so it is held apart from the effect it
// travels with — and it is the one a seat's own `keep out` deliberately does
// not trigger.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// suppressSenderDomain refuses the sender's domain a company.
//
// Hiding the mail is not enough on its own. A newsletter publisher or an
// expense tool has a real corporate website, so if a NAMED employee ever writes
// from that domain the triage reads their site, finds a genuine company, and
// creates it — the vendor arrives in the CRM by another door. The refusal has
// to be a standing decision about the domain, which is why it is recorded here
// rather than inferred from the sender each time.
//
// It never overwrites a human's admission (the store's guard), so an admin who
// let a domain in keeps it in no matter how much bulk mail follows.
//
// A free-mail domain is skipped: nobody's employer is gmail.com, so there is no
// company to refuse, and suppressing it would put a consumer mail provider in
// the admin's blocked list as though it were a decision anyone made.
// A sender the workspace CORRESPONDS with is never refused a company, whatever
// the classifier called this particular message. The two facts do not conflict:
// a supplier's marketing blast is a newsletter and the supplier is still a
// company this business works with. Hiding that one message is right; refusing
// the domain on the strength of it is not, and the refusal is the standing,
// workspace-wide half.
//
// This guard became load-bearing when the create tiers started raising verdict
// questions of their own — before that only unjudged strangers reached the
// ledger, and correspondence was already excluded by construction.
//
// corresponds is CorrespondsWith's answer for this sender, read once by the
// noise arm because the record retraction beside this call is bounded by the
// same fact — two reads could not disagree, but one read says so structurally.
func (e *CounterpartyVerdictEngine) suppressSenderDomain(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty, kind string, corresponds bool) error {
	if row.Domain == "" {
		return nil
	}
	if corresponds {
		return nil
	}
	return e.people.SuppressBulkSenderDomainTx(ctx, tx, row.Domain,
		"mail from this domain was judged "+kind+", so it is not a company this business works with")
}
