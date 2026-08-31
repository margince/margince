// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Destroying named records on a person's own authority, rather than on a
// policy's.
//
// Retention destroys what a POLICY says is too old. This destroys what an
// OWNER says was never the business's to hold: the mail their own mailbox
// brought in from a doctor, a school, a family member. The two arrive at the
// same place — the message text, its provider original, its vectors, its
// attachments, everything derived from it — and they must arrive there by the
// same code, because a second destruction path is how one of them quietly stops
// destroying something.
//
// So this reuses the retention executors verbatim and differs in exactly two
// ways: WHO decided (an owner, not a policy) and WHICH records (a list the
// caller computed, not a query over an age). Both differences ride the audit
// row, so a governance read can tell them apart.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PurgeReason names why an owner is destroying records, recorded on every audit
// row the purge writes. A closed set: an audit trail whose reason is free text
// cannot be read by anything but a human.
type PurgeReason string

const (
	// PurgeOwnerRule is an owner acting on their own exclusion rule — "keep
	// this sender out for good".
	PurgeOwnerRule PurgeReason = "owner_rule"
	// PurgePersonalVerdict is the automatic path: a sender the classifier
	// judged personal, after the undo window closed.
	PurgePersonalVerdict PurgeReason = "personal_verdict"
)

// PurgeActivities destroys the named messages and everything they left behind.
//
// One transaction per activity, not one for all of them. A purge can name
// thousands of messages, and a single transaction over that many would hold
// locks across every one of their attachments, vectors and delivery rows — so a
// failure halfway leaves the earlier ones destroyed, which is the honest
// outcome: destruction that partially succeeded has genuinely destroyed
// something, and pretending otherwise by rolling back would restore content the
// owner asked to be rid of.
//
// Returns how many were destroyed. A message already gone is not an error: the
// executor is idempotent, and a second purge of the same rule reports zero
// rather than failing.
func (s *RetentionService) PurgeActivities(ctx context.Context, ids []ids.UUID, reason PurgeReason) (int, error) {
	// Gated here rather than only at the seam that assembles the purge. This is
	// exported, so a second caller can reach it, and "the caller checked"
	// is exactly the assumption that stops being true when somebody writes that
	// second caller. Destroying mail is a person's own act or a named system
	// pass acting on a decision a person already made — never an ambient one.
	// auth.RequireHuman, which admits a person, a connector and the system
	// sweep, and refuses an AGENT — an agent has no standing to destroy
	// correspondence, and whatever it concluded reaches a person first. The
	// system arm is what lets the personal-verdict sweep carry out a decision a
	// person already made.
	//
	// Gated here rather than only at the seam that assembles the purge: this is
	// exported, so a second caller can reach it, and "the caller checked" is
	// exactly the assumption that stops being true when somebody writes that
	// second caller.
	if err := auth.RequireHuman(ctx); err != nil {
		return 0, err
	}
	// The OBJECT grant too. RequireHuman refuses an agent and reads no grant at
	// all, so on its own it would let a read-only seat destroy correspondence:
	// the sibling that erases a person for a subject request takes
	// auth.Require(ctx, "person", ActionDelete) for the same act, and this is
	// the same act on a different object.
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return 0, err
	}
	destroyed := 0
	for _, id := range ids {
		if err := s.purgeOneActivity(ctx, id, reason); err != nil {
			return destroyed, fmt.Errorf("privacy: purging a captured message: %w", err)
		}
		destroyed++
	}
	return destroyed, nil
}

// purgeOneActivity runs the retention module's own activity/erase executor and
// audits it as an owner's act.
func (s *RetentionService) purgeOneActivity(ctx context.Context, id ids.UUID, reason PurgeReason) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The SAME executor retention runs. It nulls the text, then calls
		// purgeContentDerivedFrom for the provider original, the vectors, the
		// attachments, the delivery copies and the field provenance — the list
		// that used to exist twice and was missing two entries in one copy.
		if err := s.eraseActivityContent(ctx, tx, id); err != nil {
			return err
		}
		// Audited under `erase`, the same closed verb retention uses, so the
		// governance read and the field-history scrub boundary treat the two
		// identically. What distinguishes them is the evidence: no policy, and
		// a reason naming the owner's decision.
		//
		// No addresses and no subject in the evidence. The audit row outlives
		// the message on purpose, and a purge that recorded who was written to
		// would leave the very fact the owner was destroying.
		// Spelled as a literal for the audit gate; see anonymiseOnePerson.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "erase", "activity", id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: actionErase, "purge_reason": string(reason),
		})
		if err != nil {
			return err
		}
		return storekit.EmitEventForEntity(ctx, tx, auditID, "activity", id,
			retentionAppliedPayload(actionErase, nil, nil))
	})
}

// AnonymisePeople strips the identifying columns from the named people, on an
// owner's authority.
//
// Anonymised, never deleted. A person row is referenced by activities, links
// and aggregates that a colleague may legitimately still hold, and deleting it
// would either cascade into their work or leave it pointing at nothing. What
// goes is what identifies the human: the name, the addresses, the profile
// fields. What stays is a tombstone the rest of the graph can keep referring
// to.
//
// The SAME executor retention's person/anonymize action runs, for the same
// reason PurgeActivities shares its own: two ways to anonymise a person is one
// way too many, and the one that gets less use is the one that quietly stops
// covering a column.
func (s *RetentionService) AnonymisePeople(ctx context.Context, people []ids.UUID, reason PurgeReason) (int, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return 0, err
	}
	// The same grant the subject-request eraser takes for the same act.
	if err := auth.Require(ctx, "person", principal.ActionDelete); err != nil {
		return 0, err
	}
	done := 0
	for _, id := range people {
		if err := s.anonymiseOnePerson(ctx, id, reason); err != nil {
			return done, fmt.Errorf("privacy: anonymising a purged contact: %w", err)
		}
		done++
	}
	return done, nil
}

func (s *RetentionService) anonymiseOnePerson(ctx context.Context, id ids.UUID, reason PurgeReason) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := anonymizePersonRecord(ctx, tx, id); err != nil {
			return err
		}
		// The verb spelled as a literal, not through the constant: the audit
		// gate reads call sites to tell an `update` (which must carry a
		// before-image) from a scrub verb (which must not), and a constant is
		// something it cannot resolve.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "anonymize", "person", id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: actionAnonymize, "purge_reason": string(reason),
		})
		if err != nil {
			return err
		}
		return storekit.EmitEventForEntity(ctx, tx, auditID, "person", id,
			retentionAppliedPayload(actionAnonymize, nil, nil))
	})
}

// StatutoryFloorClause is the shield every destructive activity path applies,
// handed out so a selection assembled OUTSIDE this module applies the same one.
//
// The predicate lives in retention_floor.go, and its comment says what happens
// when a path skips it: correspondence the nightly evaluator
// refuses to touch for six years gets destroyed anyway — a GoBD floor bypass.
// The capture purge is such a path, and it cannot import this module, so the
// clause travels to it rather than being written a second time.
//
// Held by: TestTheStatutoryFloorIsSpelledOnce (backend/gates/statutoryfloorsingle_test.go),
// which fails when a second file spells the window comparison.
//
// The returned fragment is the POSITIVE form: it is TRUE for an activity
// aliased `a` that the law still requires this installation to keep. A caller
// deciding what it may destroy negates it; a caller deciding what to withhold
// uses it as it stands. It expects the interval and the year-end anchor as the
// next two positional arguments, in that order; StatutoryFloorArgs supplies
// them.
func StatutoryFloorShield(intervalArg, anchorArg int) string {
	return handelsbriefShielded(intervalArg, anchorArg)
}

// StatutoryFloorArgs are the two values StatutoryFloorClause's placeholders
// take, read from the installation's configured retention period.
func StatutoryFloorArgs() (any, any) {
	return statutoryFloorArgs()
}
