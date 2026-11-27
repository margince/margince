// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The Art. 15 chapter for what this installation CONCLUDED about the subject's
// own correspondence.
//
// Its own file rather than more rows in sarsections.go, because it answers a
// different question from every other chapter there. The rest of the gather
// list exports what the subject GAVE us or what we recorded about handling
// them; this exports readings we made of their words — a verdict on what a
// reply meant, and a sentence naming what we still owe them. Art. 15 owes those
// on a footing of their own: a subject is entitled to know not only what they
// wrote but what we decided it meant, and which classifier decided it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// messageReach is the subject's own addresses and chat accounts for the export:
// the ones no other live contact also holds.
//
// A retired address or chat binding stays in the subject's package, but the
// uniqueness indexes bind only live rows. So an identifier a different live
// contact holds now belongs to them, and its mail is theirs.
type messageReach struct {
	emails, providers, accounts []string
}

func subjectOwnReach(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, identities []ids.UUID) (messageReach, error) {
	ownEmails, err := subjectStrings(ctx, tx,
		`SELECT addr FROM unnest($2::text[]) AS addr EXCEPT `+rivalHeldEmails, contactID, emails)
	if err != nil {
		return messageReach{}, fmt.Errorf("reading the subject's own addresses for the export: %w", err)
	}
	providers, accounts, err := subjectOwnChannelAccounts(ctx, tx, contactID, identities)
	if err != nil {
		return messageReach{}, err
	}
	return messageReach{ownEmails, providers, accounts}, nil
}

// subjectAuthoredMessages resolves, once per export, the activities linked to
// the subject or WRITTEN by them: the `from` participant, or the inbound sender.
// A Cc is not the author and does not earn the verdict on another's request.
//
// Shared and legally held rows stay in: Art. 15 owes them.
func subjectAuthoredMessages(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, identities []ids.UUID) ([]ids.UUID, error) {
	reach, err := subjectOwnReach(ctx, tx, contactID, emails, identities)
	if err != nil {
		return nil, err
	}
	return reach.activities(ctx, tx, contactID, []string{senderRole})
}

// subjectMessages resolves, once per export, every mail the subject sent or
// received. That is From, To, Cc or Bcc, matched by their record, address or
// chat account, plus the activities linked to them.
//
// It never reads activity_link.company_id, so a mail filed only on their
// employer stays out. A mail resolved to their record counts even on an address
// someone else now holds.
func subjectMessages(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, identities []ids.UUID) ([]ids.UUID, error) {
	reach, err := subjectOwnReach(ctx, tx, contactID, emails, identities)
	if err != nil {
		return nil, err
	}
	return reach.activities(ctx, tx, contactID, subjectMessageRoles)
}

// subjectHasMessage answers whether subjectMessages would hold a mail from the
// given source system (any, when empty), and stops at the first one found.
func subjectHasMessage(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, identities []ids.UUID, sourceSystem string) (bool, error) {
	reach, err := subjectOwnReach(ctx, tx, contactID, emails, identities)
	if err != nil {
		return false, err
	}
	var found bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id IN (`+messageArms+`)
		AND ($6 = '' OR a.source_system = $6))`,
		append(reach.args(contactID, subjectMessageRoles), sourceSystem)...).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("checking whether the subject has mail: %w", err)
	}
	return found, nil
}

// senderRole is the participant role of the one who sent a message.
const senderRole = "from"

var subjectMessageRoles = []string{senderRole, "to", "cc", "bcc"}

// messageArms holds the participant arms both selectors share. One arm per
// index, so each can use its own: the link, the participant's contact,
// lower(address), and counterparty_email. No index covers channel_user_id. That
// arm scans chat participants, as the InteractionParticipation section already
// does on every export. `UNION ALL` lets an existence check stop at the first row.
const messageArms = `
		SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1
		UNION ALL
		SELECT ap.activity_id FROM activity_participant ap WHERE ap.role = ANY($5) AND ap.contact_id = $1
		UNION ALL
		SELECT ap.activity_id FROM activity_participant ap
		 WHERE ap.role = ANY($5) AND ap.address IS NOT NULL AND lower(ap.address) = ANY($2)
		UNION ALL
		SELECT ap.activity_id FROM activity_participant ap JOIN activity a ON a.id = ap.activity_id
		 WHERE ap.role = ANY($5) AND ap.channel_user_id IS NOT NULL
		   AND (a.channel_provider, ap.channel_user_id) IN (
		         SELECT provider, account FROM unnest($3::text[], $4::text[]) AS t(provider, account))
		UNION ALL
		SELECT m.id FROM activity m
		 WHERE m.direction = 'inbound' AND m.counterparty_email IS NOT NULL AND m.counterparty_email = ANY($2)`

func (r messageReach) args(contactID ids.ContactID, roles []string) []any {
	return []any{contactID, r.emails, r.providers, r.accounts, roles}
}

func (r messageReach) activities(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, roles []string) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT activity_id FROM (`+messageArms+`) u(activity_id)`, r.args(contactID, roles)...)
	if err != nil {
		return nil, fmt.Errorf("reading the subject's messages for the export: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// subjectChannelIdentities reads the chat accounts of every identity the
// subject is, archived bindings included.
func subjectChannelIdentities(ctx context.Context, tx pgx.Tx, identities []ids.UUID) ([]channelIdentity, error) {
	rows, err := tx.Query(ctx,
		`SELECT provider, channel_user_id, account_scope FROM contact_channel_identity WHERE contact_id = ANY($1)`, identities)
	if err != nil {
		return nil, fmt.Errorf("reading the subject's chat accounts for the export: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[channelIdentity])
}

// subjectOwnChannelAccounts reads the chat accounts of every identity the
// subject is, minus any a different live contact also holds.
func subjectOwnChannelAccounts(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, identities []ids.UUID) ([]string, []string, error) {
	held, err := subjectChannelIdentities(ctx, tx, identities)
	if err != nil {
		return nil, nil, err
	}
	providers, accounts := channelIdentityPairs(held)
	rows, err := tx.Query(ctx, `
		SELECT provider, account, ''::text FROM (
			SELECT provider, account FROM unnest($2::text[], $3::text[]) AS t(provider, account)
			EXCEPT `+rivalHeldChannelAccounts+`) kept`, contactID, providers, accounts)
	if err != nil {
		return nil, nil, fmt.Errorf("dropping chat accounts another contact holds: %w", err)
	}
	own, err := pgx.CollectRows(rows, pgx.RowToStructByPos[channelIdentity])
	if err != nil {
		return nil, nil, err
	}
	providers, accounts = channelIdentityPairs(own)
	return providers, accounts, nil
}

// sarJudgementSections gather the conclusions. Each reads the one message set
// subjectAuthoredMessages resolved, as $1.
func sarJudgementSections(pkg *SARPackage, contactID ids.ContactID, authored []ids.UUID) []sarSection {
	messages := []any{authored}
	return []sarSection{
		// What we concluded the subject's replies MEANT, and who concluded it.
		// The history is exported rather than the activity's current column
		// alone, because the corrections are the half a subject cannot see any
		// other way: a rate that was moved by a rep re-judging their message is
		// a decision made about them, and the standing verdict conceals that it
		// ever happened.
		//
		// The verdict itself is NOT withheld for a limited message, where the
		// subject line beside it is. The rule those CASE arms implement is that
		// one seat's private mail must not be republished to the workspace
		// through an export; this export goes to the SUBJECT, who wrote the
		// message being judged, so withholding our conclusion about their own
		// words would hide the very holding Art. 15 asks about.
		{&pkg.ReplyJudgements, `SELECT h.verdict, h.decided_by, h.is_human, h.decided_at,
		          h.activity_id, a.occurred_at
		   FROM activity_reply_verdict_history h
		   JOIN activity a ON a.id = h.activity_id
		   WHERE h.activity_id = ANY($1)`, messages},
		// And what we concluded our own reply DID about what they asked. The
		// `remaining` sentence travels: it is the one place the package can say
		// we recorded an outstanding obligation TO the subject, and a settlement
		// exported without it would report that we judged their request and
		// withhold what we judged. The reply we read through is named by its
		// date rather than only its id, for the reason the handoff sections give
		// about labels — a bare uuid tells the subject nothing about which
		// message of ours the conclusion rests on.
		{
			&pkg.RequestSettlements, `SELECT s.verdict, s.remaining, s.due_at, s.confidence,
		          s.decided_by, s.decided_at, s.request_activity_id,
		          asked.occurred_at AS asked_at, answered.occurred_at AS answered_at
		   FROM activity_request_settlement s
		   JOIN activity asked ON asked.id = s.request_activity_id
		   LEFT JOIN activity answered ON answered.id = s.judged_through_activity_id
		   WHERE s.request_activity_id = ANY($1)`,
			messages,
		},
		{&pkg.CommitmentAdmissions, `SELECT d.locator, d.outcome, d.rule_version, d.evaluated_at,
          d.captured_by, d.source_activity_id, d.renewed_by, d.renewed_at,
          CASE WHEN ` + sarContentDisclosed + ` AND (d.renewed_by IS NULL OR EXISTS (
            SELECT 1 FROM activity a WHERE a.id = d.renewed_by AND ` + sarContentDisclosed + `
          )) THEN d.summary END AS summary,
          CASE WHEN ` + sarContentDisclosed + ` AND (d.renewed_by IS NULL OR EXISTS (
            SELECT 1 FROM activity a WHERE a.id = d.renewed_by AND ` + sarContentDisclosed + `
          )) THEN d.meaning END AS meaning,
          CASE WHEN ` + sarContentDisclosed + ` THEN d.source_quote END AS source_quote
          FROM activity_commitment_admission d JOIN activity a ON a.id = d.source_activity_id
          WHERE d.source_activity_id = ANY($1) OR d.renewed_by = ANY($1)`, messages},
		// What a deal's stage ledger read out of the subject's messages. The
		// snippet quotes the message, so it is withheld wherever the body is. The
		// criterion is named by its label, because a bare id tells the subject
		// nothing about what the claim was for. A refuted claim travels too: a
		// conclusion withdrawn later was still held about them.
		{&pkg.StageEvidence, `SELECT CASE WHEN ` + sarContentDisclosed + ` THEN ev.snippet END AS snippet,
		          ` + sarContentDisclosed + ` AS content_disclosed, ev.commitment, ev.met, ev.confidence,
		          ev.author_side, ev.extracted_by, ev.observed_at, ev.refuted_at,
		          ev.source_id AS activity_id, c.label AS criterion
		   FROM deal_stage_evidence ev
		   JOIN stage_exit_criterion c ON c.id = ev.criterion_id
		   JOIN activity a ON a.id = ev.source_id
		   WHERE ev.source_type = 'activity' AND ev.source_id = ANY($1)`, messages},
		// The claims erasure deletes: those about the subject, and those read from a
		// message they wrote or are linked to. Reading and quote follow the body.
		{&pkg.ConversationClaims, `SELECT c.kind, c.status, c.due_at, c.needs_review, c.source,
		          c.created_at, c.corrected_at, true AS derived,
		          CASE WHEN ` + sarContentDisclosed + ` THEN c.body END AS reading,
		          CASE WHEN ` + sarContentDisclosed + ` THEN c.source_quote END AS quote,
		          ` + sarContentDisclosed + ` AS content_disclosed,
		          c.source_activity_id AS activity_id, a.occurred_at
		   FROM conversation_claim c
		   JOIN activity a ON a.id = c.source_activity_id
		   WHERE c.contact_id = $1 OR c.source_activity_id = ANY($2)`, []any{contactID, authored}},
	}
}
