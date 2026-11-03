// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signature-enrich pass (ADR-0063). A contact whose newest readable inbound
// mail this pass has not read gets one model read of its signature block. While no job title is stated or held, up to two older mails
// are read for one. Every read is evidence-or-omit: gateEvidence drops a field
// whose snippet is not verbatim in the supplied lines, and the confidence floor
// is 0.6.
//
// What lands is decided by RECENCY, not by emptiness: a signature is the contact
// stating their own details on a date, so a later one replaces what the record
// holds — a colleague's typed value included, because a number stated last week
// outranks one typed in March. The replaced value is kept for one click of undo.
//
// The single exception is a field a human CORRECTED: they read the machine's
// answer and ruled on it, and correctedAt below is how this pass defers to that
// ruling instead of re-inferring over it.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// companyNameField is the profile field a captured employer name lands in.
const companyNameField = "company_name"

const (
	// signatureLineCount is the §2.9 input pin: the trailing non-quoted
	// lines of the one inbound mail being read.
	signatureLineCount = 15
	// enrichConfidenceFloor is the §2.9 acceptance floor.
	enrichConfidenceFloor = 0.6
	// enrichPassLimit bounds one pass's candidate set. A pass that fills it
	// queues a continuation rather than leaving the remainder to the nightly
	// cycle; see RunWorkspace.
	enrichPassLimit = 100
)

// enrichFieldNames is the §2.9 closed vocabulary, shared with the card reader: a
// signature and a card state the same things about a contact, and two vocabularies
// would let which one arrived decide what could be recorded.
//
// `title` is the only word for a job title. `role` named the same thing and is gone:
// only title is mirrored onto contact.title (contacts.observedFieldColumn), so a value
// filed under role left the column empty and the screen showed a dash for a signature
// that stated it outright — and offering both let the model pick differently run to run.
// The contract keeps role for the research-claim surface and the rows already carrying
// it; TestSignatureEnrichOffersNoSecondWordForAJobTitle holds this.
var enrichFieldNames = nameSet(signatureFieldNames)

// signatureFieldNames lists the vocabulary once, in the order the user prompt
// offers it. The name is not a field: the reading reports its parts under
// "name", and contacts.MapSignatureName decides which becomes first_name and
// last_name.
var signatureFieldNames = []string{fieldTitle, fieldPhone, "linkedin", companyNameField, "address", "website"}

func nameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// signatureRepeatedFields hold a list. A signature listing a German, a
// Vietnamese and a Singapore number states three numbers, all current, and a
// gate that kept the first would record one and lose two.
var signatureRepeatedFields = map[string]bool{"phone": true}

const signatureEnrichSystem = `You extract contact fields from ONE email signature. Allowed fields ONLY: title, phone,
linkedin, company_name, address, website. A job title is always title. Emit a field ONLY if the signature lines state it verbatim; the snippet
must appear character-for-character in the supplied text. Ignore quoted replies, legal
disclaimers, and marketing taglines. Phone numbers verbatim, never normalized. A signature
often lists several numbers (desk, mobile, one per country): emit one phone entry for EVERY
number, each with its own snippet; never merge them and never keep only one.
Emit address as the single line the signature prints it on. Emit website only for the
company's own site; a social profile is never a website, and linkedin carries that one.
The signature must be THE NAMED CONTACT'S OWN. A block naming somebody else — a colleague,
a forwarded sender, a correspondent quoted underneath — states nothing about them, so emit
no fields at all rather than the ones it happens to contain.
Report the contact's name in the separate "name" object, never as a field, and only from how
this contact names THEMSELF: the name printed in their own signature block, or the one they
sign off with above it. Its keys are family_name, given_names, middle (empty when there is
none), order, evidence_snippet and confidence. Copy each part as printed and report the order
you SAW: given_first when the given names are printed before the family name, family_first
when the family name is printed first, unclear when the page does not show which word is the
family name. Never decide the order from where a word stands, and never reorder the words.
"Lê Thị Mai" is family_name "Lê", middle "Thị", given_names "Mai", order family_first.
"Anna Weber" is given_names "Anna", family_name "Weber", order given_first. When the order
is unclear, leave family_name and given_names empty. evidence_snippet is the printed line
that holds the name. A company, a brand, a team or a department is never a name, so a block
headed "Greven Ebusiness" or "Seven Mills Consulting" states no name. Omit "name" when the
lines show nobody naming themself.`

// signatureEnrichSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func signatureEnrichSystemFor(fence promptfence.Fence) string {
	return signatureEnrichSystem + "\n" + fence.Rule("signature")
}

// CaptureEnricher drives the signature pass for every workspace.
type CaptureEnricher struct {
	pool  *pgxpool.Pool
	store *contacts.Store
	brain completer
	log   *slog.Logger
	// limit is how many candidates one pass takes, and therefore the count at
	// which it reports a continuation is due. enrichPassLimit in production; a
	// test sets it small so it can drive that boundary with a handful of
	// contacts rather than a hundred.
	limit int
}

// NewCaptureEnricher builds the pass over the pool and one model lane.
func NewCaptureEnricher(pool *pgxpool.Pool, brain completer, log *slog.Logger) *CaptureEnricher {
	return &CaptureEnricher{
		pool:  pool,
		store: newContactsStore(InstallationDB(pool)),
		brain: brain,
		log:   log,
		limit: enrichPassLimit,
	}
}

// RunWorkspace enriches up to enrichPassLimit candidates in the workspace
// already bound in ctx. A budget stop ends the pass cleanly; per-contact model
// trouble is logged and the contact is retried next cycle (their watermark did
// not move).
//
// It reports whether the pass filled its limit AND moved at least one contact,
// which is the caller's signal that more contacts are due right now. Two things
// make that worth acting on rather than leaving to the nightly pass. A mailbox
// sync lands hundreds of messages at once, so the 101st contact would wait
// until tonight for details a rep is about to read. And the trigger's own
// uniqueness means mail arriving mid-pass dedupes against a pass that has
// already listed its candidates — so without a continuation that message waits
// for the nightly run too, whatever the queue depth.
//
// Both halves are load-bearing; see the return statement for why progress
// alone bounds the chain.
//
// A full set is not proof that anyone is left: the last pass may have finished
// exactly at the limit. The continuation costs one job that lists no candidates
// and stops, which is the cheap side of the trade.
func (e *CaptureEnricher) RunWorkspace(ctx context.Context) (filled bool, err error) {
	// ONE PASS AT A TIME, or two of them pay for the same page.
	//
	// The candidate list is a plain read, so two passes select the same contacts,
	// make the same model calls and each spend for them. Three doors can put a
	// pass in flight — the nightly tick, the arrival trigger and a pass's own
	// continuation — and the first two dedupe against each other on the queue's
	// uniqueness window while the third deliberately cannot: it is enqueued from
	// inside a RUNNING job with the same args, so a window that suppressed it
	// would end the chain the continuation exists to keep.
	//
	// So the mutual exclusion is here, where the spending is. Taken and released
	// rather than waited on: a pass that finds another running has nothing to
	// add — the holder is reading the same candidates now — and its successor
	// will be queued by whoever is holding it.
	held, release, err := e.holdThePass(ctx)
	if err != nil {
		return false, err
	}
	if !held {
		e.log.InfoContext(ctx, "signature enrich: another pass holds this installation, so this one stands down")
		return false, nil
	}
	defer release()
	// The store's apply writes audit + outbox rows, so the pass binds
	// the system actor and an operation scope like every worker job.
	wsCtx := principal.SystemActing(ctx, "agent:enrich")
	// The workspace default, for every mailbox that never made its own choice.
	// Read once per pass rather than per candidate: it is one row, and a value
	// that changed mid-pass would sort one night's candidates by two different
	// answers to the same question.
	defaultEnrich, err := config.StoredVal(wsCtx, StoredConfig(e.pool), capture.SignatureEnrich)
	if err != nil {
		return false, fmt.Errorf("reading the signature-enrichment setting: %w", err)
	}
	// The rows an earlier pass wrote off somebody else's signature, taken back
	// before this one looks for candidates. It runs every pass rather than once
	// in a migration because the two halves must not be able to disagree: the
	// same senderPredicate decides what may be written below and what may still
	// stand here, so a change to one is a change to both.
	if removed, err := e.store.RetractMisattributedSignatureFields(wsCtx); err != nil {
		return false, err
	} else if removed > 0 {
		e.log.InfoContext(wsCtx, "signature enrich: took back fields read off a message the contact did not send",
			"fields", removed)
	}
	candidates, err := e.store.SignatureCandidates(wsCtx, e.limit, defaultEnrich)
	if err != nil {
		return false, err
	}
	// A candidate that fails is logged, not returned, and that survives the
	// fan-out deliberately: the retry is durable in the DATA rather than in
	// River. A contact whose verdict could not be parsed wrote no watermark,
	// so SignatureCandidates re-selects them on the next pass. Failing
	// the workspace row for one unparseable reply would retry the whole
	// candidate set to re-reach the same contact.
	var advanced int
	for _, cand := range candidates {
		if err := e.enrichOne(wsCtx, cand, defaultEnrich); err != nil {
			if ai.IsDeferral(err) {
				e.log.InfoContext(wsCtx, "signature enrich: work deferred, stopping the pass")
				// No continuation: the deferral is what stopped this pass, and a
				// follow-on would hit the same wall immediately. The nightly
				// cycle is the right owner of work the budget deferred.
				return false, nil
			}
			e.log.WarnContext(wsCtx, "signature enrich: candidate failed",
				"contact", cand.ContactID.String(), "err", err)
			continue
		}
		advanced++
	}
	// A continuation needs BOTH a full set and somebody moved off it.
	//
	// The full set alone would chain forever. A failed candidate writes no
	// watermark on purpose — the comment above says why — so it stays the
	// newest mail and fills the next pass too. Queueing on the count alone,
	// a hundred candidates whose model call keeps failing would re-select
	// themselves, enqueue another job, and do it again: an unbroken chain of
	// jobs that each succeed, spend the model budget, and starve every older
	// contact behind them. River's attempt cap cannot stop it, because no job
	// in the chain ever fails.
	//
	// Requiring progress bounds the chain by the work actually done. A pass
	// that moved nobody is the end of it, and the nightly cycle owns the
	// candidates it could not read — which is where they lived before this
	// continuation existed.
	return advanced > 0 && len(candidates) == e.limit, nil
}

// unparseableReply reports the one drop reason that means the model failed
// rather than the signature being silent — the distinction the read cursor
// turns on.
func unparseableReply(dropped []droppedFinding) bool {
	for _, d := range dropped {
		if d.Reason == dropUnparseableReply {
			return true
		}
	}
	return false
}

// signatureHistoryDepth is how many of a candidate's newest readable mails one
// pass may read, the newest included. The older ones are read only for a
// contact who still has no title, and the walk stops at the first title stated.
const signatureHistoryDepth = 3

// enrichOne reads the candidate's newest mail and, while no title is stated or
// held, up to signatureHistoryDepth-1 older ones. Only the newest mail moves
// the watermark. A model error anywhere returns before it, so the contact stays
// a candidate.
func (e *CaptureEnricher) enrichOne(ctx context.Context, cand contacts.SignatureCandidate, defaultEnrich bool) error {
	titled, err := e.readSignature(ctx, cand, cand.ActivityID, signatureReadOptions{DefaultEnrich: defaultEnrich})
	if err != nil {
		return err
	}
	if !titled && cand.OlderMailMayFillTitle {
		older, err := e.store.OlderSignatureSources(ctx, cand.ContactID, cand.ActivityID, signatureHistoryDepth-1, defaultEnrich)
		if err != nil {
			return err
		}
		for _, source := range older {
			if titled, err = e.readSignature(ctx, cand, source, signatureReadOptions{TitleOnly: true, DefaultEnrich: defaultEnrich}); err != nil || titled {
				break
			}
		}
		if err != nil {
			return err
		}
	}
	return e.store.MarkSignatureRead(ctx, cand.ContactID, cand.ActivityID, signatureHistoryDepth)
}

type signatureReadOptions struct {
	TitleOnly     bool
	DefaultEnrich bool
}

// Older mail may fill only an unclaimed title. Readability is checked before
// the model call and again when applying fields, because access may change.
func (e *CaptureEnricher) readSignature(ctx context.Context, cand contacts.SignatureCandidate, source ids.UUID, options signatureReadOptions) (bool, error) {
	body, readable, err := e.store.ReadableSignatureBody(ctx, cand.ContactID, source, options.DefaultEnrich)
	if err != nil || !readable {
		return false, err
	}
	// A copy of the candidate naming this mail, so the fenced source_id and the
	// evidence gate name the mail being read.
	mail := cand
	mail.ActivityID, mail.Body = source, body
	lines := signatureBlock(mail.Body)
	if !signatureNamesContact(lines, mail) {
		// Their message, somebody else's signature at the foot of it: grounded
		// text with the wrong owner, so nothing is read from it.
		e.log.DebugContext(ctx, "signature enrich: the block does not identify this contact, so nothing is read from it",
			"contact", mail.ContactID.String(), "reason", dropSignatureNotThisContact)
		return false, nil
	}
	// The request carries this contact's own signature block, so the call is
	// about the contact rather than about the message it was cut from.
	req := signatureEnrichRequest(mail, lines)
	about := ai.WithSubject(ctx, mail.ContactID.Ref(), mail.FullName)
	resp, err := ai.Ask(about, e.brain, req, signatureShapeValid)
	if err != nil {
		return false, err
	}

	// The code-side gate: verbatim-snippet-or-drop against the exact lines
	// the model was shown, then the confidence floor.
	gated, dropped := gateEvidence(resp.Text, lines, "activity:"+mail.ActivityID.String(),
		func(name string) bool { return enrichFieldNames[name] }, signatureRepeatedFields)
	if unparseableReply(dropped) {
		// A reply no reader can parse is a fault in the ANSWER, not evidence
		// that the signature states nothing — so the read goes unrecorded and
		// this contact is asked again next pass.
		return false, fmt.Errorf("compose: unparseable signature reply for contact %s", mail.ContactID)
	}
	if len(dropped) > 0 {
		e.log.DebugContext(ctx, "signature enrich: fields dropped by the evidence gate",
			"contact", mail.ContactID.String(), "dropped", len(dropped))
	}
	fields := make([]contacts.SignatureField, 0, len(gated)+2)
	if !options.TitleOnly {
		nameFields, nameDropped := gateSignatureName(resp.Text, lines)
		if len(nameDropped) > 0 {
			e.log.DebugContext(ctx, "signature enrich: the name was not written",
				"contact", mail.ContactID.String(), "reason", nameDropped[0].Reason)
		}
		fields = append(fields, nameFields...)
	}
	for _, f := range gated {
		if float64(f.Confidence) < enrichConfidenceFloor || (options.TitleOnly && f.Field != fieldTitle) {
			continue
		}
		fields = append(fields, contacts.SignatureField{
			Name: f.Field, Value: f.Value, Evidence: f.EvidenceSnippet, Confidence: float64(f.Confidence),
			// The claim key the correction ledger stores for this field. The
			// apply re-reads the ledger under its own lock and defers to any
			// ruling it finds; computing the key here is what lets it, because
			// the key is a hash only this module can build.
			ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath(f.Field)),
		})
	}
	titled := slices.ContainsFunc(fields, func(f contacts.SignatureField) bool {
		return f.Name == fieldTitle && strings.TrimSpace(f.Value) != ""
	})
	switch {
	case len(fields) == 0:
		return false, nil
	case options.TitleOnly:
		_, err = e.store.FillSignatureTitle(ctx, mail.ContactID, mail.ActivityID, fields[0], options.DefaultEnrich)
	default:
		_, err = e.store.ApplySignatureFields(ctx, mail.ContactID, mail.ActivityID, fields, options.DefaultEnrich)
	}
	return titled, err
}

// enrichPassLock names the lock the pass holds, as text rather than as a
// number: an advisory key is an integer namespace shared by the whole database,
// and a bare constant is how two unrelated passes come to hold one key.
const enrichPassLock = "capture_signature_enrich_pass"

// holdThePass takes the pass's session lock. A worker that dies drops its
// connection and Postgres drops the lock with it. A `held_until` column would
// keep the next pass out until the clock caught up.
func (e *CaptureEnricher) holdThePass(ctx context.Context) (bool, func(), error) {
	release, held, err := holdSessionLock(ctx, e.pool, enrichPassLock, e.log)
	return held, release, err
}

// signatureNamesContact asks whether the block a field was read from names the
// contact it is about to be written to.
//
// The candidate query already requires this contact to have SENT the message, so
// this is the second shape: their own mail whose foot carries somebody else's
// signature — a forwarded footer, a colleague's block under theirs, a
// correspondent's details quoted without the ">" that would have excluded them.
// The value is verbatim in the window in every one of those cases, so the
// evidence gate passes it and only the NAME can tell whose it is.
//
// A WHOLE-WORD match on a name token, or the address.
//
// Whole word rather than substring, because a substring is satisfied by an
// unrelated name that happens to contain one: "Joanne Brown" contains "ann", so
// a signature naming Joanne would have been read as Ann Smith's own. That is
// the very defect this function exists to refuse, reached through the check
// meant to catch it.
//
// Every token counts, however short. An earlier version skipped tokens under
// three characters so particles ("de", "van") could not match — but with the
// whole-word test a particle only matches a particle, and skipping short ones
// permanently locked out anybody whose name is short in the first place ("Li
// Bo" signing "Li Bo\nCEO" is a signature nobody could ever read).
func signatureNamesContact(block string, cand contacts.SignatureCandidate) bool {
	folded := strings.ToLower(block)
	if cand.Email != "" && strings.Contains(folded, strings.ToLower(cand.Email)) {
		return true
	}
	words := make(map[string]bool)
	for _, word := range strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		words[word] = true
	}
	// Two tokens when the name has two: a surname alone is shared by relatives
	// and colleagues, and the name gate writes from this block.
	var tokens, matched int
	for token := range strings.FieldsSeq(strings.ToLower(cand.FullName)) {
		token = strings.Trim(token, ".,;:()<>\"'")
		if token == "" {
			continue
		}
		tokens++
		if words[token] {
			matched++
		}
	}
	return matched > 0 && matched >= min(tokens, 2)
}
