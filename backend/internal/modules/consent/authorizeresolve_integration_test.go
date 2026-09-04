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
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true}},
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

// resolve asks the engine what a message is, for this env's subject.
func (e *resolveEnv) resolve(t *testing.T, req commsauthz.Request) resolution {
	t.Helper()
	var out resolution
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.resolveCategory(context.Background(), tx, req, subjectRef{
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
		stranger, err = e.gate.resolveCategory(context.Background(), tx,
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
