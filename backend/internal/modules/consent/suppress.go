// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Recording that we may not write to somebody.
//
// The engine has read communication_suppression since it landed, privacy
// deletes from it on erasure and the subject access export returns it — and
// until now NOTHING in production wrote a row. A rep who learned by phone that
// a contact wants no more mail had nowhere to put that, so the only way the
// table filled was a migration carrying old withdrawals across.
//
// A row here is not the absence of consent. It outranks a grant, it does not
// expire on its own, and re-granting must not silently erase it — which is why
// this is its own verb rather than a consent state.
//
// WHO DECIDED IS PART OF THE RECORD. A rep writing this records a `user`-level
// row, which an admin may lift and another rep may not. The level is taken from
// the authenticated principal and never from the request: a body that could
// name its own authority would let any caller write a row nobody can lift.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// SuppressInput is a rep saying we may not write to this person.
type SuppressInput struct {
	// PersonID is the only subject this door takes. A lead can be suppressed by
	// the same table and is not reachable here: leads have no consent surface
	// yet, and an input field naming a subject the authorization check cannot
	// name would be a door claiming a reach it does not have.
	PersonID ids.PersonID
	// Kind is which stop this is. The vocabulary is the table's own CHECK, and
	// a caller may only ever record subject_request — the other three are
	// written by machinery (a bounce) or describe a legal fact (an objection, a
	// restriction) that a rep relaying a phone call is not recording.
	Kind string
	// Reason is what the rep was told, in their words. Stored because a
	// suppression somebody later wants lifted is only reviewable if the record
	// says why it was made.
	Reason string
}

// The kinds a seat may write, and the two are a different shape of thing.
//
// subject_request is "they asked us to stop contacting them" — broad, and the
// scope the subject actually named is whatever the rep was told.
//
// marketing_objection is Art. 21(2)/(3): an objection to direct marketing. It
// is deliberately writable BY HAND, because the article gives the subject an
// unconditional right exercised "at any time" and by any means. Requiring a
// signed form or a self-service link before one could be recorded would put a
// condition on an unconditional right, and the practical effect was worse than
// the theory: nothing wrote this kind at all, so a rep told "stop the
// newsletter" on the phone had only subject_request — which stopped the
// invoices too.
//
// A processing restriction and a hard bounce stay machine-written: the first is
// an Art. 18 legal state with its own workflow, the second a fact about a
// mailbox only the mail path can observe.
var suppressibleKinds = []string{"subject_request", commsauthz.ReasonObjection}

// suppressibleKind is the kind a seat writes when the subject asked to stop
// being contacted at all, as opposed to objecting to marketing.
const suppressibleKind = "subject_request"

// auditFieldKind is the wire request body's own field name, named so a
// validation refusal names the field the caller actually sent.
const auditFieldKind = "kind"

// auditFieldSuppressionKind is a DIFFERENT name from the wire field above,
// deliberately: the audit payload's key is read back by historyFieldLabel
// (frontend/src/screens/historyfieldlabels.ts) with no entity context, and
// "kind" already names activities' own audited create field
// (backend/internal/modules/activities/activity.go's fieldKind) — the same
// bare key on two writers would mislabel whichever ships second.
const auditFieldSuppressionKind = "suppression_kind"

// Suppress records that a subject asked not to be written to.
//
// The verb is deliberately narrow: it writes one row, at the caller's own
// authority level, and it does not touch consent. A withdrawal and a
// suppression are different acts — one says "I take back this permission", the
// other "stop writing to me" — and collapsing them would make a re-grant
// silently undo a stop the subject never lifted.
func (s *Store) Suppress(ctx context.Context, in SuppressInput) error {
	sub, level, err := admitSuppress(ctx, in)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.suppressAdmittedTx(ctx, tx, in, sub, level)
	})
}

// admitSuppress settles everything decidable before a connection is taken: the
// subject, the kind, and that this caller may write about that subject at all.
func admitSuppress(ctx context.Context, in SuppressInput) (subject, commsauthz.AuthorityLevel, error) {
	sub, err := consentSubject(RecordInput{PersonID: in.PersonID})
	if err != nil {
		return subject{}, "", err
	}
	if !slices.Contains(suppressibleKinds, in.Kind) {
		return subject{}, "", &ValidationError{
			Field: auditFieldKind,
			Reason: "a person may record that the subject asked us to stop, or objected to " +
				"marketing; a processing restriction and a bounce are not recorded by hand here",
		}
	}
	// The BOUND only. The contract makes this reason optional (`required: [kind]`),
	// and a rep relaying a phone call may have nothing to add — what it must not
	// be is a megabyte every later reader of this person's history is served.
	if err := boundReason(in.Reason); err != nil {
		return subject{}, "", err
	}
	// "person" as a literal rather than sub.entityType, which is the same value
	// on every path this door has: SuppressInput reaches it from a person route
	// only. A computed object name is a door no static check can resolve, and
	// grantreachability_test.go counts those — an authorization gate nothing can
	// scan is exactly the one worth keeping scannable.
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	return sub, levelFor(ctx, in.Kind), nil
}

// levelFor answers whose decision this stop is, which is what decides who may
// later lift it.
//
// AN OBJECTION IS THE SUBJECT'S ACT, whoever typed it. Art. 21 gives the right
// to the data subject; a rep relaying the phone call is a courier, not the
// author, and recording their seat level would let another seat of equal or
// greater rank lift a stop the subject asked for. CanOverrule refuses to rank
// anything above LevelSubject, so stamping it here is what makes the objection
// hold against the whole staff.
//
// The cost is real and accepted: a mistyped objection can be undone only by the
// subject reversing it or by the per-message exception path. That is the right
// way round — the failure of a stop that is too hard to lift is an awkward
// conversation, and the failure of one that is too easy is mail somebody
// explicitly refused.
//
// subject_request keeps the seat's own level: it is the rep's report of a
// conversation, with no article behind it, and an admin correcting a
// misheard "stop everything" should not need the subject on the phone.
func levelFor(ctx context.Context, kind string) commsauthz.AuthorityLevel {
	if kind == commsauthz.ReasonObjection {
		return commsauthz.LevelSubject
	}
	return authorityOf(ctx)
}

// authorityOf reads the caller's tier from the authenticated principal.
//
// Never from the request. A body naming its own level would let any caller
// write a `subject`-level row, which nothing in the installation can lift —
// a denial of service against one contact, spelled as a permission.
//
// auth.RequireAdmin is the literal admin role, so an `ops` seat lands at
// LevelUser. That is deliberate and matches the RBAC defaults, where ops and
// admin are separated by exactly this gate — see LevelAdmin's own comment.
func authorityOf(ctx context.Context) commsauthz.AuthorityLevel {
	if auth.RequireAdmin(ctx) == nil {
		return commsauthz.LevelAdmin
	}
	return commsauthz.LevelUser
}

// suppressAdmittedTx writes the row, its audit entry and its event together.
func (s *Store) suppressAdmittedTx(
	ctx context.Context, tx pgx.Tx, in SuppressInput, sub subject, level commsauthz.AuthorityLevel,
) error {
	// auth.EnsureWritable, the same probe recordAdmittedTx runs for a
	// withdrawal — row scope, capture privacy and write authority together. A
	// bare existence check would let somebody stop a contact they cannot open:
	// `person` carries capture privacy, so a mailbox sync's unpromoted rows are
	// invisible to every seat but their owner's, and a stop written onto one is
	// a refusal its owner can neither see nor explain.
	//
	// EnsureWritable and not EnsureWritableLive, for the reason the withdrawal
	// arm gives: a suppression stays recordable against an archived — including
	// an Art. 17 anonymized — subject. A stop is what you most want to survive.
	if err := auth.EnsureWritable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	// storekit.CapturedBy, which is the write shape's own reader: it resolves
	// the authenticated principal to the string every audited table stores, so
	// a suppression names its author the same way every other row does.
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	// ON CONFLICT DO NOTHING would be wrong here: a second request is a second
	// occasion somebody asked, and the reason they gave may differ. The live
	// index makes a duplicate harmless — liveSuppression takes the strongest
	// row — so the honest answer is to record both.
	if _, err = tx.Exec(ctx, `
		INSERT INTO communication_suppression
		    (`+sub.column+`, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, $3, $4, $5)`,
		sub.id, in.Kind, suppressionSource(in.Reason), by, string(level)); err != nil {
		return fmt.Errorf("consent: recording the suppression: %w", err)
	}

	// "update" on the person, which is what qualifyingevent.go records for the
	// same class of write — a human changing what may be sent to somebody.
	//
	// NOT "restrict", which looks apt and is a trap: privacy/fieldhistory.go
	// treats it as a SCRUB TOMBSTONE, so a suppression audited that way would
	// hide every earlier edit to that person as though their data had been
	// erased. The record history would also render it "System withheld the
	// record", which is not what happened.
	// AuditEvent and not Audit: this write has no prior state to image. A
	// suppression is a new fact about the person, not an edit to a field that
	// held something before, and Audit refuses an update with no before-image
	// rather than let one record a change it cannot describe.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id,
		map[string]any{auditFieldSuppressionKind: in.Kind, "decided_by_level": string(level)})
	if err != nil {
		return err
	}
	// EmitEvent, because the payload declares a STATIC entity: this door writes
	// about a person and only a person, so the fan-out gate resolves delivery
	// scope from the type rather than from a runtime subject somebody has to
	// ratify by hand.
	return storekit.EmitEvent(ctx, tx, auditID, sub.id, suppressionRecordedPayload(in.Kind, level))
}

// suppressionSource carries what the rep was told, bounded.
//
// `source` is free text describing WHERE the row came from, so an empty reason
// still records the door rather than an empty string somebody later reads as a
// missing field.
func suppressionSource(reason string) string {
	if reason == "" {
		return "recorded by a person"
	}
	return "recorded by a person: " + reason
}

// suppressionRecordedPayload names what was recorded and at which authority.
//
// Deliberately not the reason. That is the subject's own words about
// themselves, and an event fans out further than the row it describes — a
// consumer that never needed to read a phone call should not receive one.
func suppressionRecordedPayload(kind string, level commsauthz.AuthorityLevel) crmcontracts.PublicEventConsentSuppressed {
	return crmcontracts.PublicEventConsentSuppressed{
		Kind:           kind,
		DecidedByLevel: string(level),
	}
}
