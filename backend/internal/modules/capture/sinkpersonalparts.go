// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a private thread's files are allowed to leave behind.
//
// The read boundary already keeps a colleague out of personal mail, and
// erasure reaches the bytes. This is minimisation rather than correctness: not
// storing a payslip, a medical letter or a school form is strictly better than
// storing it and being able to delete it later, and a thread the classifier
// has already judged private is exactly the case where the product knows it
// does not want the file.
//
// The cost, stated rather than glossed: the owner loses the file LIST too. A
// row naming the attachment without its bytes would need the keeper contract
// to carry a withheld file, and that shape belongs to the module that owns the
// attachment table. Until then the breadcrumb is what says the files were kept
// out deliberately rather than never seen — which is the distinction somebody
// asking "where are my attachments" actually needs.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withheldPersonalParts is the breadcrumb a stripped message leaves, in the
// same vocabulary a bounded one uses.
const withheldPersonalParts = "capture_personal_parts_withheld"

// fieldVerdict names WHICH act made the thread private: the classifier's
// reading, or the seat's own hand.
const fieldVerdict = "verdict"

// stripPersonalParts takes a private thread's files out of the record before
// anything stores them, and reports how many it withheld.
//
// The parts are REMOVED rather than emptied. Staging writes each body to the
// object store unconditionally, so an emptied part would put a zero-length
// object there — storing something, under a key the message then points at,
// which is worse than either keeping the file or keeping nothing.
//
// The count is what makes this answerable afterwards. A message that arrives
// with no parts and one whose parts were withheld look identical on the row;
// the breadcrumb is the only thing that separates "this had none" from "this
// had three and we deliberately kept none".
func stripPersonalParts(rec connector.NormalizedRecord) (connector.NormalizedRecord, int) {
	withheld := len(rec.Parts)
	rec.Parts = nil
	return rec, withheld
}

// personalPartsWithheld records that a thread's files were kept out, once per
// message and with no filename, type or size in it.
//
// A sender controls all three, and system_log is operational: the count is
// ours, the names are theirs.
//
// The VERDICT rides along because the two ways a thread becomes private are
// different acts. `held` is the classifier's reading, which somebody may
// disagree with; `held_by_owner` is the seat saying so by hand, which nobody
// overrules. A breadcrumb that credited the classifier for both would misstate
// half of them, and the half it misstates is the one where a human decided.
func (s *Sink) personalPartsWithheld(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, count int, verdict string,
) error {
	if count == 0 {
		return nil
	}
	return s.logBreadcrumbTx(ctx, tx, withheldPersonalParts, rec,
		"this thread is private",
		map[string]any{fieldDroppedParts: count, fieldVerdict: verdict})
}
