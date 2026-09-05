// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the engine concludes a message IS, from the record rather than from the
// label its sender put on it.
//
// Integration because every answer here is a join: whether this recipient wrote
// into the thread being answered, whether a linked deal is open and they are a
// stakeholder on it. A unit test could only assert that a function returns what
// it was told, which is the thing under test.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type resolveEnv struct {
	gate     *Gate
	store    *Store
	ctx      context.Context
	owner    *pgx.Conn
	ws, user ids.UUID
	person   ids.PersonID
	address  string
	// A deal needs a pipeline and a stage, so the env carries one of each
	// rather than every deal fixture making its own.
	pipeline, stage ids.UUID
}

func setupResolve(t *testing.T) *resolveEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &resolveEnv{
		ws: ids.NewV7(), user: ids.NewV7(),
		person: ids.New[ids.PersonKind](), address: "dana@buyer.test",
		owner: owner,
	}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.user, "rep-"+e.user.String()+"@r.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Dana Buyer', 'manual', 'human:x')`, e.person); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:x')`, e.person, e.address); err != nil {
		t.Fatal(err)
	}

	e.pipeline, e.stage = ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO pipeline (id, name, is_default) VALUES ($1, 'Sales', true)`,
		e.pipeline); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualifying', 1)`,
		e.stage, e.pipeline); err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	e.gate = NewGate(e.store)

	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	e.ctx = principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			// The grants a rep who sends about a document actually holds. The
			// engine now probes a named invoice or contract for readability
			// before it reads one, so a fixture without these is testing the
			// refusal rather than the validator.
			Objects: map[string]principal.ObjectGrant{
				"person":   {Read: true},
				"finance":  {Read: true},
				"contract": {Read: true},
				// The installation's own settings, which is what the
				// disagreement report is gated on: it discloses nothing about
				// any subject, only how two rules have compared.
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return e
}

// inboundFrom records a message the subject SENT us, on a named thread.
func (e *resolveEnv) inboundFrom(t *testing.T, threadKey, sender string, when time.Time) ids.UUID {
	t.Helper()
	return e.activity(t, threadKey, "inbound", when, sender, "from")
}

// outboundTo records a message we sent, on a named thread, with the subject as
// a recipient rather than the writer.
func (e *resolveEnv) outboundTo(t *testing.T, threadKey, recipient string, when time.Time) ids.UUID {
	t.Helper()
	return e.activity(t, threadKey, "outbound", when, recipient, "to")
}

// activityWithoutThread plants a message that never joined a thread, which is
// a real shape: thread_key is nullable.
func (e *resolveEnv) activityWithoutThread(t *testing.T, direction, address, role string) ids.UUID {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now(), 'gmail', 'human:x')`, id, direction); err != nil {
		t.Fatalf("planting the unthreaded activity: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, person_id, address, role)
		VALUES ($1, $2, $3, $4)`, id, e.person, address, role); err != nil {
		t.Fatalf("planting the participant: %v", err)
	}
	return id
}

func (e *resolveEnv) activity(t *testing.T, threadKey, direction string, when time.Time, address, role string) ids.UUID {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, $3, $4, 'gmail', 'human:x')`,
		id, direction, threadKey, when); err != nil {
		t.Fatalf("planting the activity: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, person_id, address, role)
		VALUES ($1, $2, $3, $4)`, id, e.person, address, role); err != nil {
		t.Fatalf("planting the participant: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, id, e.person); err != nil {
		t.Fatalf("linking the activity: %v", err)
	}
	return id
}

// openDeal plants a live opportunity, and makes the subject a stakeholder on it
// unless stakeholder is false.
func (e *resolveEnv) openDeal(t *testing.T, status string, stakeholder bool) ids.UUID {
	t.Helper()
	ctx := context.Background()
	dealID := ids.NewV7()
	var closedAt *time.Time
	if status != "open" {
		now := time.Now()
		closedAt = &now
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO deal (id, name, pipeline_id, stage_id, status, closed_at, source, captured_by)
		VALUES ($1, 'A deal', $2, $3, $4, $5, 'manual', 'human:x')`,
		dealID, e.pipeline, e.stage, status, closedAt); err != nil {
		t.Fatalf("planting the deal: %v", err)
	}
	if stakeholder {
		if _, err := e.owner.Exec(ctx, `
			INSERT INTO relationship (kind, deal_id, person_id, source, captured_by)
			VALUES ('deal_stakeholder', $1, $2, 'manual', 'human:x')`, dealID, e.person); err != nil {
			t.Fatalf("planting the stakeholder: %v", err)
		}
	}
	return dealID
}

// seedPurpose plants a legacy consent purpose of a named class.
func (e *resolveEnv) seedPurpose(t *testing.T, key, class string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, $2, $2, $3, false)`,
		ids.New[ids.PurposeKind](), key, class); err != nil {
		t.Fatalf("planting the purpose: %v", err)
	}
}

// decide runs the WHOLE per-recipient decision, not just the resolution — which
// is what the Resolved/Requested split has to be asserted through, because the
// split happens in decideResolved rather than in the resolver.
func (e *resolveEnv) decide(t *testing.T, req commsauthz.Request) commsauthz.Decision {
	t.Helper()
	var out commsauthz.Decision
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.decideOne(e.ctx, tx,
			connector.Recipient{Email: e.address}, req, commsauthz.PhaseStaging)
		return err
	}); err != nil {
		t.Fatalf("deciding: %v", err)
	}
	return out
}

// resolve asks the engine what a message is, for this env's subject.
func (e *resolveEnv) resolve(t *testing.T, req commsauthz.Request) resolution {
	t.Helper()
	var out resolution
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.resolveCategory(e.ctx, tx, req, subjectRef{
			Kind: entityPerson, ID: e.person.String(), Address: e.address,
		})
		return err
	}); err != nil {
		t.Fatalf("resolving: %v", err)
	}
	return out
}

// A reply to a thread the subject started is a reply, on the strength of the
// thread alone. This is the answer the old purpose model could not reach: it
// saw a consent_purpose and had no way to know the subject wrote first.
func TestAReplyToAThreadTheSubjectStartedIsAReply(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if got.Category != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved %q, want reply_to_inbound", got.Category)
	}
	if !got.Supported {
		t.Error("the thread does not support the reply, want it supported on the subject's own message")
	}
	if got.Basis != commsauthz.BasisSubjectInitiatedCorrespondence {
		t.Errorf("basis = %q, want subject_initiated_correspondence", got.Basis)
	}
}

// A reply anchored on a LATER message in the same thread is still a reply. A
// rep answering the third mail in an exchange anchors on that mail, and the
// subject may have written only the first — asking whether they wrote THIS
// message would refuse an entirely ordinary reply.
func TestAReplyIsAboutTheThreadNotTheOneMessage(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-48*time.Hour))
	ours := e.outboundTo(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: ours})

	if !got.Supported || got.Category != commsauthz.CategoryReplyToInbound {
		t.Fatalf("resolved %q supported=%v, want a supported reply_to_inbound", got.Category, got.Supported)
	}
}

// BEING COPIED IS NOT WRITING. A recipient who only ever appeared in the To
// line of somebody else's message has initiated nothing, and treating that as
// subject-initiated correspondence would let anyone manufacture a lawful basis
// for writing to a third party by putting them in Cc.
//
// Mutation: drop the role = 'from' clause and this passes with a bare copy
// recipient counted as a correspondent.
func TestBeingCopiedOnAThreadIsNotWritingIntoIt(t *testing.T) {
	e := setupResolve(t)
	// An inbound message somebody ELSE wrote, on which our subject is a 'to'.
	anchor := e.activity(t, "thread-1", "inbound", time.Now().Add(-time.Hour), e.address, "to")

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if got.Supported && got.Category == commsauthz.CategoryReplyToInbound {
		t.Fatal("a recipient who was merely copied resolved as a reply, want no thread support")
	}
}

// A DIFFERENT thread's inbound does not support this reply. Thread continuity
// is what the anchor establishes; a message the subject sent about something
// else is not an invitation to answer on this one.
func TestAnInboundOnAnotherThreadDoesNotSupportThisReply(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "thread-other", e.address, time.Now().Add(-time.Hour))
	anchor := e.outboundTo(t, "thread-1", e.address, time.Now())

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if got.Supported && got.Category == commsauthz.CategoryReplyToInbound {
		t.Fatal("an unrelated thread supported this reply, want it unsupported")
	}
}

// AN ANCHOR WITH NO THREAD MATCHES NOTHING. thread_key is nullable, so a
// message that never joined a thread has NULL there — and SQL would happily
// join every other threadless activity to it if the query did not say
// otherwise. That would make one unthreaded inbound anywhere in the
// installation support a reply to anybody.
//
// Mutation: drop the `anchor.thread_key IS NOT NULL` guard and this passes with
// an unrelated threadless message counted as the subject writing in.
func TestAnAnchorWithNoThreadSupportsNothing(t *testing.T) {
	e := setupResolve(t)
	// The subject wrote something, but on no thread at all.
	e.activityWithoutThread(t, "inbound", e.address, "from")
	// And the message being answered is likewise threadless.
	anchor := e.activityWithoutThread(t, "outbound", e.address, "to")

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if got.Supported && got.Category == commsauthz.CategoryReplyToInbound {
		t.Fatal("a threadless anchor matched a threadless inbound, want no thread support")
	}
}

// An anchor that does not exist, or was archived, supports nothing. A caller
// naming a stale activity id must not fall through to an allow.
func TestAMissingAnchorSupportsNothing(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: ids.NewV7()})

	if got.Supported {
		t.Fatal("an anchor that does not exist supported the message")
	}
}

// A live deal the recipient is a stakeholder on supports an unprompted
// follow-up.
func TestALiveDealTheRecipientIsAStakeholderOnSupportsAFollowUp(t *testing.T) {
	e := setupResolve(t)
	deal := e.openDeal(t, "open", true)

	got := e.resolve(t, commsauthz.Request{Links: []ids.UUID{deal}})

	if got.Category != commsauthz.CategoryActiveDealFollowup {
		t.Errorf("resolved %q, want active_deal_followup", got.Category)
	}
	if !got.Supported {
		t.Error("an open deal with the recipient as stakeholder did not support the follow-up")
	}
}

// BOTH HALVES ARE REQUIRED. A live deal the recipient has nothing to do with
// does not make them writable-to — that is the shape that turns one opportunity
// into a licence to mail everyone at the company. And a stakeholder row on a
// closed deal is history: the opportunity that justified the follow-up is over.
func TestADealSupportsAFollowUpOnlyWhenLiveAndTheRecipientIsOnIt(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      string
		stakeholder bool
	}{
		{"open deal, recipient is not a stakeholder", "open", false},
		{"closed deal, recipient is a stakeholder", "won", true},
		{"closed deal, recipient is not a stakeholder", "won", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			deal := e.openDeal(t, tc.status, tc.stakeholder)

			got := e.resolve(t, commsauthz.Request{Links: []ids.UUID{deal}})

			if got.Supported && got.Category == commsauthz.CategoryActiveDealFollowup {
				t.Fatal("resolved as a supported deal follow-up, want it unsupported")
			}
		})
	}
}

// THE THREAD OUTRANKS THE CLAIM. A rep who mislabels a reply still gets a
// reply: the subject wrote to us and has not withdrawn, which is the strongest
// ground a message can have, so it is checked before anything the caller said.
func TestAThreadOutranksAMistakenClaim(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	got := e.resolve(t, commsauthz.Request{
		AnchorActivityID: anchor,
		Context:          commsauthz.CategoryMarketing,
	})

	if got.Category != commsauthz.CategoryReplyToInbound || !got.Supported {
		t.Fatalf("resolved %q supported=%v, want the thread to win over the claim", got.Category, got.Supported)
	}
}

// A CLAIM THE RECORD DOES NOT BEAR OUT BECOMES A REVIEW, and it keeps the
// caller's own category. A rep who says "this is an invoice" and has no invoice
// should be told that; being silently re-labelled as marketing and refused for
// want of consent would send them looking in the wrong place entirely.
func TestAnUnsupportedClaimIsReviewedUnderItsOwnName(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryInvoiceOrPayment})

	if got.Category != commsauthz.CategoryInvoiceOrPayment {
		t.Errorf("resolved %q, want the claimed invoice_or_payment kept for the reader", got.Category)
	}
	if got.Supported {
		t.Error("an invoice claim with no invoice was supported")
	}
	if got.Reason != commsauthz.ReasonNoEvidence {
		t.Errorf("reason = %q, want no_compatible_evidence", got.Reason)
	}
}

// THE ESCAPE HATCH IS CLOSED. The legacy transactional purpose was an
// unconditional allow, so any message calling itself operational was one. It
// now resolves to an account notice that is NOT supported, and says exactly
// why — the disagreement is recorded while the old gate still decides.
func TestTheLegacyTransactionalPurposeNoLongerCarriesItself(t *testing.T) {
	e := setupResolve(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, 'transactional', 'Transactional', 'transactional', false)`,
		ids.New[ids.PurposeKind]()); err != nil {
		t.Fatal(err)
	}

	got := e.resolve(t, commsauthz.Request{LegacyPurposeKey: "transactional"})

	if got.Category != commsauthz.CategoryAccountNotice {
		t.Errorf("resolved %q, want account_notice", got.Category)
	}
	if got.Supported {
		t.Fatal("the transactional purpose supported itself, which is the escape hatch this closes")
	}
	if got.Reason != commsauthz.ReasonLegacyTransactionalUnevidenced {
		t.Errorf("reason = %q, want legacy_transactional_unevidenced", got.Reason)
	}
}

// A recipient the engine can say nothing about resolves to marketing and is
// unsupported — the strictest reading, because an unknown purpose is not a
// reason to assume an operational one.
func TestAnUnknownPurposeResolvesStrictly(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{LegacyPurposeKey: "no-such-purpose"})

	if got.Supported {
		t.Fatal("an unknown purpose was supported")
	}
	if got.Reason != commsauthz.ReasonUnknownPurpose {
		t.Errorf("reason = %q, want unknown_purpose", got.Reason)
	}
}

// The engine's answer is still per recipient. Two people on one message, one on
// the thread and one not, get different answers — which is what makes a refusal
// explainable to the person it is about.
func TestTwoRecipientsOnOneMessageGetTheirOwnAnswers(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	onThread := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})
	if !onThread.Supported {
		t.Fatal("the thread participant was not supported")
	}

	var stranger resolution
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		stranger, err = e.gate.resolveCategory(e.ctx, tx,
			commsauthz.Request{AnchorActivityID: anchor}, subjectRef{
				Kind: entityPerson, ID: ids.New[ids.PersonKind]().String(),
				Address: "stranger@elsewhere.test",
			})
		return err
	}); err != nil {
		t.Fatalf("resolving the stranger: %v", err)
	}
	if stranger.Supported {
		t.Fatal("somebody who never wrote into the thread was supported by it")
	}
}

// A CLAIM MUST NOT STEER AUTHORITY. Decision.Resolved selects which rollout
// mode applies and whether the jurisdiction's advertising ceiling is counted at
// all, so a caller whose claim set it would hold both: a marketing send
// claiming active_deal_followup would skip the ceiling, and its own decision row
// would then not count against the next send either — degrading the ceiling for
// that address permanently.
//
// The claim is recorded in Requested, which is the column that exists for it.
// Resolved stays with what the engine worked out for itself.
//
// Mutation: revert BOTH guards together — assign d.Resolved from the resolution
// before checking Supported, AND drop legacyVerdictFor's own assignment from
// the purpose class. Either alone leaves this green, because the other covers
// it; that is deliberate defence in depth rather than a redundant line, and it
// is written here so a reader who deletes one and sees a green suite knows the
// second is now the only thing holding it.
func TestAnUnsupportedClaimIsRecordedButNeverResolvedTo(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")

	d := e.decide(t, commsauthz.Request{
		LegacyPurposeKey: "newsletter",
		Context:          commsauthz.CategoryActiveDealFollowup,
	})

	if d.Resolved == commsauthz.CategoryActiveDealFollowup {
		t.Fatal("the caller's unproven claim became the resolved category, which is what the ceiling and the rollout mode key off")
	}
	if d.Resolved != commsauthz.CategoryMarketing {
		t.Errorf("resolved %q, want the engine's own reading of the purpose: marketing", d.Resolved)
	}
	if d.Requested != commsauthz.CategoryActiveDealFollowup {
		t.Errorf("requested = %q, want the claim recorded for the reader", d.Requested)
	}
}

// A SUPPORTED resolution does set Resolved — it is the engine's own conclusion
// from the record, not a claim. Without this the test above would pass with an
// engine that never resolved anything.
func TestASupportedResolutionSetsTheResolvedCategory(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor})

	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Fatalf("resolved %q, want reply_to_inbound from the thread itself", d.Resolved)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Errorf("verdict = %q, want an allow on the subject's own message", d.Verdict)
	}
}

// A STAKEHOLDER WHO WAS REMOVED supports nothing. Removing a contact from a
// deal writes archived_at; ended_at is a business date somebody types and stays
// NULL. Checking only ended_at would let a removed stakeholder keep supporting
// mail about the opportunity they are no longer on.
//
// Mutation: drop `r.archived_at IS NULL` and this fails.
func TestARemovedStakeholderSupportsNoFollowUp(t *testing.T) {
	e := setupResolve(t)
	deal := e.openDeal(t, "open", true)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE relationship SET archived_at = now() WHERE deal_id = $1`, deal); err != nil {
		t.Fatal(err)
	}

	got := e.resolve(t, commsauthz.Request{Links: []ids.UUID{deal}})

	if got.Supported {
		t.Fatal("a stakeholder edge that was archived still supported a follow-up")
	}
}

// A ROLE MAILBOX THAT MOVED does not carry its previous holder's thread. The
// bare-address arm exists for a participant capture never resolved to a record;
// without a person_id IS NULL guard it matches ANY row carrying the address, so
// info@ re-pointed from one contact to another would let the first person's
// messages support writing to the second — past their own withdrawal.
//
// Mutation: drop `p.person_id IS NULL` from the address arm and this fails.
func TestAReassignedAddressDoesNotInheritTheThread(t *testing.T) {
	e := setupResolve(t)
	// Somebody ELSE wrote into the thread from the address our subject now holds.
	previous := ids.New[ids.PersonKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Previous Holder', 'manual', 'human:x')`, previous); err != nil {
		t.Fatal(err)
	}
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'thread-1', now(), 'gmail', 'human:x')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, person_id, address, role)
		VALUES ($1, $2, $3, 'from')`, id, previous, e.address); err != nil {
		t.Fatal(err)
	}
	anchor := e.outboundTo(t, "thread-1", e.address, time.Now())

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if got.Supported {
		t.Fatal("the previous holder's message supported writing to the address's new owner")
	}
}

// EACH RECIPIENT'S CLAIM IS THEIR OWN. Staging writes one decision per
// recipient, all in one transaction, so every row carries the same decided_at —
// a single row taken for the whole delivery is a tie the planner breaks however
// it likes, and one recipient's claim would then judge everybody.
//
// Mutation: read one row for the delivery (ORDER BY decided_at DESC LIMIT 1)
// instead of DISTINCT ON the recipient, and this fails.
func TestEachRecipientsStagedClaimIsRecoveredSeparately(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	// Two recipients on one delivery, staged in one transaction with different
	// claims — exactly what AuthorizeStagingTx writes.
	e.plantStagingDecision(t, delivery, "first@corp.test", commsauthz.CategoryInvoiceOrPayment)
	e.plantStagingDecision(t, delivery, "second@corp.test", commsauthz.CategoryMarketing)

	var claims map[string]stagedClaim
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		claims, err = stagedClaims(context.Background(), tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("reading the staged claims: %v", err)
	}

	if got := claims["first@corp.test"].category; got != commsauthz.CategoryInvoiceOrPayment {
		t.Errorf("first recipient's claim = %q, want invoice_or_payment", got)
	}
	if got := claims["second@corp.test"].category; got != commsauthz.CategoryMarketing {
		t.Errorf("second recipient's claim = %q, want marketing — not the other recipient's", got)
	}
}

// A recipient with no staged claim gets none, rather than inheriting somebody
// else's. This is also the shape of a delivery staged before the engine
// existed, which must keep sending exactly as it did.
func TestARecipientWithNoStagedClaimInheritsNothing(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	e.plantStagingDecision(t, delivery, "first@corp.test", commsauthz.CategoryInvoiceOrPayment)

	var claims map[string]stagedClaim
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		claims, err = stagedClaims(context.Background(), tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("reading the staged claims: %v", err)
	}

	req := stagedRequestFor(commsauthz.TransmitRequest{PurposeKey: "newsletter"},
		connector.Recipient{Email: "nobody@corp.test"}, claims)
	if req.Context != "" {
		t.Fatalf("an unstaged recipient inherited the claim %q", req.Context)
	}
	if req.LegacyPurposeKey != "newsletter" {
		t.Errorf("purpose key = %q, want the delivery's own", req.LegacyPurposeKey)
	}
}

// plantDelivery makes a delivery row for decisions to hang off.
func (e *resolveEnv) plantDelivery(t *testing.T) ids.UUID {
	t.Helper()
	ctx := context.Background()
	activityID := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'email', 'outbound', 'manual', now(), 'human:x')`, activityID); err != nil {
		t.Fatal(err)
	}
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		   consent_purpose, references_chain, status)
		VALUES ($1, $2, $3, 'gmail', $4, '[]'::jsonb, '[]'::jsonb, 'S', 'b',
		        'newsletter', '[]'::jsonb, 'pending')`,
		id, activityID, e.user, "msg-"+id.String()); err != nil {
		t.Fatal(err)
	}
	return id
}

// plantStagingDecision writes one staging row the way AuthorizeStagingTx does:
// attempt 0, and now() for decided_at, so every row of one delivery ties.
func (e *resolveEnv) plantStagingDecision(t *testing.T, delivery ids.UUID, address string, claimed commsauthz.Category) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   requested_category, resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 0, $2, $3, 'staging', $4, 'marketing', 'review', 'no_compatible_evidence', 'observe', 'test')`,
		delivery, ids.NewV7(), address, string(claimed)); err != nil {
		t.Fatalf("planting the staging decision: %v", err)
	}
}

// issueLinkRow plants a confirm_token in one of the shapes the validator must
// tell apart: live, spent, or expired.
//
// Written directly rather than through Store.issueLink, and this is the one
// place in this file where that is right. issueLink mints a token AND stages the
// mail that carries it, so seeding through it would need the whole controller
// lane wired — a stager, a vault, a job runner — to set up a case about whether
// the row supports a category. What the validator reads is four columns, and
// this writes exactly those four in each of the shapes the real writer produces:
// consumed_at NULL or set, expires_at ahead or behind. A shape the writer cannot
// produce would be caught by the column list disagreeing with the schema.
//
//nolint:unparam // kind is the axis TestAConfirmationLinkDoesNotSupportTheOtherKind varies; a helper fixed to one kind could not express that case
func (e *resolveEnv) issueLinkRow(t *testing.T, kind string, expires time.Time, consumed *time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO confirm_token (id, person_id, token_hash, delivered_to, expires_at, consumed_at, kind)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, e.person, "hash-"+id.String(), e.address, expires, consumed, kind); err != nil {
		t.Fatal(err)
	}
	return id
}
