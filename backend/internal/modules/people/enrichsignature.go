// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The signature-enrich apply half (ADR-0063, §2.9): the model's gated fields
// land here, by RECENCY — a signature is the contact stating their own details
// on a date, so a later one replaces what the record holds and keeps the
// replaced value for one click of undo. Every accepted field upserts its
// PO-DDL-12 evidence row so the value stays auditable back to the verbatim
// signature line, and the field-provenance stamp rides the same commit.
//
// The rule the recency does not override is a human's correction, which the
// caller supplies as SignatureField.CorrectedAt.
//
// org_name is evidence-only: it NEVER creates or renames an organization (the
// deterministic domain path owns employer derivation).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// enrichSource is the DM-CONV-11 channel for signature-extracted fields.
const enrichSource = "capture_enrich"

// enrichCapturedBy is the acting identity on enrichment rows.
const enrichCapturedBy = "agent:enrich"

// SignatureField is one gated, evidence-carrying extraction.
type SignatureField struct {
	Name       string // title | phone | role | linkedin | org_name | address | website
	Value      string
	Evidence   string // verbatim snippet — the caller's gate already verified it
	Confidence float64
	// CorrectedAt is when a human last corrected THIS field, when one has. It
	// is the one thing a newer statement does not outrank: somebody read the
	// machine's answer and ruled on it, and the contract promises them it will
	// not be silently re-inferred. Nil where nobody has ruled.
	//
	// Supplied by the caller because the ruling lives in another module's
	// ledger, which this one may not read.
	CorrectedAt *time.Time
}

// SignatureApplyResult counts what one apply did — honest numbers for the
// digest, not fabrications.
type SignatureApplyResult struct {
	Applied int // fields this statement was newer than, and therefore replaced
	Skipped int // fields answered later than this, or ruled on by a human
}

// ApplySignatureFields lands one person's gated signature fields in one
// transaction. A field lands when this statement is NEWER than what the record
// holds, and every one that lands writes its evidence row and provenance stamp.
// A field is counted skipped when the record already carries something stated
// later, or when a human's correction stands against it.
func (s *Store) ApplySignatureFields(ctx context.Context, personID ids.PersonID, sourceActivity ids.UUID, fields []SignatureField) (SignatureApplyResult, error) {
	var res SignatureApplyResult
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return res, err
	}
	if len(fields) == 0 {
		return res, nil
	}
	sourceRef := "activity:" + sourceActivity.String()
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// LIVE, not merely visible: SignatureCandidates selects a live person,
		// a model call runs, and this applies afterwards —
		// auth.EnsureWritableLive says why that window is the write's own
		// obligation rather than the entry gate's.
		//
		// writePersonProfileField carries the same liveness on its INSERT and
		// that is not this: it answers "nothing landed", which this pass counts
		// as a field somebody had already filled. The probe is what refuses an
		// erased subject at the door instead of counting it skipped. It does
		// not cover the subject going mid-transaction — the writer's row lock
		// serializes that, and this pass still reports it as a skip, which is
		// the honest count for a field that did not land.
		//
		// HOLD rather than probe, because the source check below takes a lock of
		// its own. The eraser is subject-first (privacy/erasure.go anonymizes
		// the subject's rows before deleting what hangs off it), so the subject
		// must be the first row this transaction holds or the two deadlock and
		// an erasure fails when it loses.
		if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// The SOURCE has the same window as the subject, and for the same
		// reason: SignatureCandidates selected an open message, a model call
		// ran, and a human or a verdict can limit that message before this
		// lands. What this writes is a title, a phone and an employer onto a
		// person every seat reads, and the audience rescope deliberately does
		// not retract profile fields — so a field landed after the narrowing
		// stays readable for good, which is the one outcome no later correction
		// reaches.
		//
		// Nothing landed rather than an error: this is the same shape as a
		// field somebody else had already filled, and the pass counts it a skip.
		// A narrowed source is not a fault, it is an answer that arrived too
		// late.
		//
		// Archived counts with the other two: the candidate query requires
		// `a.archived_at IS NULL`, so a message archived during the model call
		// is one this pass would no longer select, and copying its text onto a
		// person afterwards outlives the archive that was meant to retire it.
		//
		// The test is for a source that IS limited, not for the absence of an
		// open one. A source row that is gone says nothing about an audience —
		// it was erased, or the caller named a message this store never had —
		// and refusing on absence would make the write depend on a row the
		// contract never promised, which is a different rule wearing this one's
		// clothes.
		// FOR SHARE, so the answer cannot go stale between this statement and
		// the field writes below. Read-committed would otherwise let a
		// narrowing commit in that gap and the fields land anyway — and these
		// fields are the ones no later correction reaches, because the audience
		// rescope does not retract them.
		// occurred_at rides the same read: it is WHEN the person stated these
		// values, and the writer compares it against what the record already
		// holds. Taken from the message and not the clock, because this pass
		// may run days after the mail arrived and re-delivery is ordinary — a
		// processing time would let a replayed old signature outrank a recent
		// one.
		var sourceLimited bool
		var observedAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT audience <> 'workspace'
			       OR restricted_at IS NOT NULL
			       OR archived_at IS NOT NULL,
			       occurred_at
			  FROM activity WHERE id = $1 FOR SHARE`,
			sourceActivity).Scan(&sourceLimited, &observedAt); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("people: reading the signature's source message: %w", err)
			}
			// No source row: nothing lands. The fields being applied were read
			// OUT of that message, so a source that is gone — erased since the
			// candidate was selected — means writing erased content onto a
			// person every seat reads, into fields the audience rescope does
			// not retract and an audit trail that is append-only. Absence is
			// the one answer that cannot be checked, so it fails closed.
			sourceLimited = true
		}
		if sourceLimited {
			res.Skipped += len(fields)
			return nil
		}
		var appliedFields []string
		// Every field this pass landed, as it found it and as it left it —
		// keyed by the field name, whether it is a column of the person or a
		// row of person_profile_field. The site and search fills record the
		// sidecar fields the same way, and a field that projected as a change
		// from one writer and as nothing from another would give one field two
		// histories.
		before, after := map[string]any{}, map[string]any{}
		for _, f := range fields {
			verdict, err := s.applySignatureField(ctx, tx, personID, sourceRef, observedAt, f)
			if err != nil {
				return err
			}
			if !verdict.applied {
				res.Skipped++
				continue
			}
			res.Applied++
			appliedFields = append(appliedFields, f.Name)
			// Named, not quoted. This pass parses its values out of a message
			// somebody sent, and one of the fields it can fill is a phone
			// number; audit_log is append-only, so a value written here outlives
			// the erasure that clears the record it came from. The same refusal
			// the bought-claim writers make, for the same reason and the same
			// closed vocabulary of field names.
			//
			// nil rather than the replaced value, even though this pass can now
			// replace one: the before image is subject to the same refusal as
			// the after image. What was there is recoverable from the field
			// row's own undo buffer, which erasure clears with the record.
			before[f.Name] = nil
			after[f.Name] = signatureFieldFilled
		}
		if len(appliedFields) == 0 {
			return nil
		}
		// The write shape: the enrichment is a person mutation, so the
		// audit row and the person.updated outbox event ride this commit.
		//
		// WHICH mail the signature came from, and which fields landed at all,
		// are context ABOUT the mutation and ride evidence: anything placed in
		// the images is projected by field history as a change to a field of
		// that name (storekit.AuditWithEvidence).
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityPerson, personID.UUID,
			before, after, map[string]any{
				auditKeySource: enrichSource, auditKeyFields: appliedFields, auditKeySourceRef: sourceRef,
			})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, personID.UUID, crmcontracts.PublicEventPersonUpdated{
			ChangedFields: map[string]any{auditKeyFields: appliedFields, auditKeySource: enrichSource},
		})
	})
	if err != nil {
		return SignatureApplyResult{}, err
	}
	return res, nil
}

// readSignatureValue is the candidate value a signature field contributes, in
// the shape the column it fills accepts — or false when this pass cannot read
// one, which is a skipped field and not a failure.
//
// The phone goes through values.ParsePhone, the same door the create and dedupe
// paths use. person_phone.phone is E.164 by contract and nothing in the database
// enforces it, so a signature line — which states a number in whatever shape its
// author types, "+49 (30) 1234-5678" or "030 12345678" — would reach the column
// verbatim. The one without a country prefix is the one that matters: it is
// unreachable, and it defeats the phone dedupe, which matches the normalized
// form.
//
// It normalizes BEFORE the evidence row is written rather than at the INSERT,
// because the sidecar's `value` column is what the surfaces show as the claim
// this evidence supports. Normalizing only the person_phone row would leave the
// two disagreeing about the same fact.
func readSignatureValue(f SignatureField) (string, bool) {
	value := strings.TrimSpace(f.Value)
	if value == "" {
		return "", false
	}
	if f.Name != "phone" {
		return value, true
	}
	parsed, err := values.ParsePhone(value)
	if err != nil {
		// Declined, not failed: a footer this pass cannot read is one candidate
		// skipped, exactly like an empty one. Propagating it would abandon the
		// other fields of the same signature, and the rest of the batch, over
		// one person's formatting.
		return "", false
	}
	return parsed.String(), true
}

// signatureVerdict is what one gated field did to the record: whether it landed
// at all, and the value it landed — which is the value the audit image carries,
// normalized, rather than the string the signature spelled.
type signatureVerdict struct {
	applied bool
	value   string
}

// signatureFieldFilled marks a field this pass answered. The image says WHICH
// field moved and does not carry what it moved to.
const signatureFieldFilled = "filled"

func (s *Store) applySignatureField(ctx context.Context, tx pgx.Tx, personID ids.PersonID, sourceRef string, observedAt time.Time, f SignatureField) (signatureVerdict, error) {
	value, readable := readSignatureValue(f)
	if !readable {
		return signatureVerdict{}, nil
	}

	// The person's own dated statement, applied by the writer both this pass
	// and the card import share. A phone goes to the number list, because a
	// contact has several and a signature names one of them; everything else is
	// a single answer that the newer statement replaces.
	if f.Name == "phone" {
		outcome, err := applyObservedPhone(ctx, tx, personID, observedPhone{
			Phone: value, PhoneType: emailTypeWork, SourceRef: sourceRef,
			Source: enrichSource, CapturedBy: enrichCapturedBy, ObservedAt: observedAt,
		})
		if err != nil || outcome != observedApplied {
			return signatureVerdict{}, err
		}
	} else {
		outcome, err := applyObservedField(ctx, tx, personID, observedField{
			Field: f.Name, Value: value, Evidence: f.Evidence, SourceRef: sourceRef,
			Source: enrichSource, CapturedBy: enrichCapturedBy, Confidence: &f.Confidence,
			ObservedAt: observedAt, CorrectionAt: f.CorrectedAt,
		})
		if err != nil || outcome != observedApplied {
			return signatureVerdict{}, err
		}
	}

	if err := storekit.StampFields(ctx, tx, entityPerson, personID.UUID, sourceRef, enrichCapturedBy,
		[]storekit.FieldStamp{{Field: f.Name}}); err != nil {
		return signatureVerdict{}, err
	}
	return signatureVerdict{applied: true, value: value}, nil
}

// revokeSignatureEvidence withdraws the just-inserted evidence row when
// its guarded column fill lost the race: evidence must never claim a
// value the record does not carry.
func revokeSignatureEvidence(ctx context.Context, tx pgx.Tx, personID ids.PersonID, field string) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM person_profile_field
		WHERE person_id = $1 AND field = $2 AND source = $3`,
		personID, field, enrichSource); err != nil {
		return fmt.Errorf("people: withdrawing unapplied signature evidence (%s): %w", field, err)
	}
	return nil
}

// MarkSignatureRead advances the read watermark to THIS mail, whatever the read
// yielded. It is what stops a person whose signature states nothing the pass
// wants from being re-read — and re-billed — every night: they return as a
// candidate only when mail NEWER than the watermark arrives.
//
// The watermark is the message's own occurred_at and it only moves forward
// (GREATEST): a cursor that could rewind would reopen the person as soon as the
// mail it names is archived, or whenever two workers finished out of order.
//
// Bookkeeping, not a domain mutation, so it carries no audit or outbox row — the
// same posture as the capture auto-enrich cursor. Deliberately outside the apply
// transaction too: losing the watermark after a successful apply costs one
// repeated read whose evidence rows are all first-verdict-wins inserts, so the
// repeat changes nothing.
func (s *Store) MarkSignatureRead(ctx context.Context, personID ids.PersonID, activityID ids.UUID) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// occurred_at is read from the activity rather than taken from the
		// caller: the watermark and the candidate query must compare the same
		// number, and only one of them can be the row itself.
		tag, err := tx.Exec(ctx, `
			INSERT INTO person_signature_enrich_state (person_id, activity_id, last_activity_at)
			SELECT $1, a.id, a.occurred_at FROM activity a
			 WHERE a.id = $2 AND a.restricted_at IS NULL
			ON CONFLICT (person_id) DO UPDATE
			SET activity_id = EXCLUDED.activity_id,
			    last_activity_at = GREATEST(person_signature_enrich_state.last_activity_at,
			                                EXCLUDED.last_activity_at),
			    attempted_at = now()`,
			personID, activityID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// The activity is gone (erased, archived away) between the candidate
			// query and here. Nothing to watermark against; the next pass reads
			// whatever mail this person has now.
			return nil
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("people: recording the signature read: %w", err)
	}
	return nil
}

// SignatureCandidate is one person the enrich pass should look at, and the
// mail to read their signature from.
type SignatureCandidate struct {
	PersonID   ids.PersonID
	FullName   string
	Email      string
	ActivityID ids.UUID
	Body       string // the latest inbound email's stored body
}

// SignatureCandidates lists the people whose latest inbound mail this pass has
// not read yet, each with that message — bounded for one pass.
//
// Candidacy is arrival, not absence. A person whose title and phone are already
// on the record is still a candidate when new mail comes in, because a contact
// who changed jobs or numbers says so in a signature and the record is stale
// until somebody reads it. This is what the fill-only-empty predicate that used
// to sit here could not express: it asked "is anything missing", which stops
// being true after the first answer and never becomes true again.
//
// Asking is bounded by person_signature_enrich_state: the pass watermarks the
// mail it read, and a person returns only when mail NEWER than the watermark
// arrives. That is what keeps the cost proportional to correspondence rather
// than to the size of the address book — one model call per person per new
// message, however often the pass runs. The comparison is on occurred_at, not on the activity id — an
// identity check would reopen the person the moment the mail it names is
// archived, and the query would then pay to re-read an OLDER signature.
// defaultEnabled is the workspace answer for a mailbox that never made its own
// choice, and the caller reads it from the capture settings rather than this
// package — people owns neither the setting nor the connection.
func (s *Store) SignatureCandidates(ctx context.Context, limit int, defaultEnabled bool) ([]SignatureCandidate, error) {
	var out []SignatureCandidate
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.full_name, coalesce(pe.email, ''), a.id, coalesce(a.body, '')
			FROM person p
			LEFT JOIN person_email pe ON pe.person_id = p.id AND pe.is_primary AND pe.archived_at IS NULL
			JOIN LATERAL (
				SELECT a.id, a.body, a.occurred_at, a.captured_by
				FROM activity_link al
				JOIN activity a ON a.id = al.activity_id
				WHERE al.person_id = p.id AND al.entity_type = 'person'
				  AND a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
				  -- A limited message is not signature material. What this pass
				  -- extracts — a title, a phone, an employer — is written onto a
				  -- person every seat can read, so mining a message whose
				  -- audience excludes those seats republishes its content in
				  -- field form, and narrowing the mail afterwards does not take
				  -- the field back. The candidate simply waits for open mail.
				  AND a.audience = 'workspace'
				  -- A row held under a statutory obligation is out of reach of
				  -- every ordinary read (A165/ADR-0114 §2); the model call this
				  -- feeds is processing, which is what the hold bars.
				  AND a.restricted_at IS NULL
				ORDER BY a.occurred_at DESC
				LIMIT 1
			) a ON true
			LEFT JOIN person_signature_enrich_state st ON st.person_id = p.id
			-- Anyone the workspace still corresponds with, and no longer only
			-- the people whose details are missing. A contact changes jobs and
			-- numbers, so a filled field is a question this pass must keep
			-- asking; the emptiness test that used to live here would answer it
			-- once and never again. The watermark below is what keeps that
			-- affordable: it is arrival of NEW mail, not absence of an answer,
			-- that makes somebody a candidate.
			WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL
			  AND (st.last_activity_at IS NULL OR a.occurred_at > st.last_activity_at)
			  -- The mailbox that captured THIS mail decides whether it may be
			  -- read for a signature, and the test is here rather than in the
			  -- Go loop so a switched-off mailbox never consumes a slot of the
			  -- pass's own limit. Gated on the activity, not on p.captured_by:
			  -- a person captured by one mailbox is regularly last written to
			  -- from another, and it is the mail being read that matters.
			  --
			  -- The join is the provenance string capture stamps
			  -- (connector:<provider>:<user id>); there is no foreign key. A row
			  -- stamped with the bare connector:<name> form — no granting user
			  -- bound — matches no connection and follows the workspace default,
			  -- which is the same answer it had before this switch existed.
			  AND COALESCE((
				SELECT cc.signature_enrich_enabled
				  FROM capture_connection cc
				 WHERE ('connector:' || cc.provider || ':' || cc.user_id::text) = a.captured_by
				   AND cc.archived_at IS NULL
			  ), $2)
			-- Freshest mail first. The pass is capped, and it now runs within
			-- minutes of a message arriving, so ordering by person age would
			-- put a contact who just wrote behind every older one still waiting
			-- — the person whose details a rep is about to look at is the one
			-- who would wait longest.
			ORDER BY a.occurred_at DESC
			LIMIT $1`, limit, defaultEnabled)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c SignatureCandidate
			if err := rows.Scan(&c.PersonID, &c.FullName, &c.Email, &c.ActivityID, &c.Body); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("people: listing signature candidates: %w", err)
	}
	return out, nil
}
