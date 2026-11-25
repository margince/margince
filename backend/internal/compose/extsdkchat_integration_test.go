// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A member-owned chat source of an extension unit, end to end over a real
// Postgres. A neutral unit stores a member's hand-filed message in its own
// table and asks for an unattended run with SyncNow. The run ingests it on that
// member's authority, filed on a contact or lead the member chose.
//
// The unit goes through RegisterExtensions, so it passes the admission a shipped
// unit does. Its job handler runs on a Runtime minted the way a tick's is.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/unitsql/unitsqltest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	chatUnit      = "memberchat"
	chatSystem    = "member-chat"
	chatProvider  = "member_chat"
	chatJob       = "ingest"
	sdkChatSource = "ext:" + chatUnit + ":" + chatSystem
	chatTable     = "ext.ext_memberchat_filed"
)

// filedMessage is what the unit's route stores for a member's hand-filed message.
type filedMessage struct {
	Member, Key, Account, Text string
	Precision                  extension.OccurredPrecision
	SubjectKind                extension.SubjectKind
	SubjectID                  string
	Files                      []extension.InboundFile
}

// fileByHand is the unit's route body: store the message, then ask for a run.
func fileByHand(ctx context.Context, rt extension.Runtime, msg filedMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO `+chatTable+` (id, body) VALUES (gen_random_uuid(), $1)`, string(body))
		return err
	}); err != nil {
		return err
	}
	return rt.SyncNow(ctx, chatJob)
}

func (m filedMessage) record() extension.Record {
	rec := extension.Record{
		System: chatSystem, Key: m.Key, ThreadKey: chatSystem + ":" + m.Account,
		Activity: extension.ActivityFields{
			Kind: extension.ActivityKindMessage, ChannelProvider: chatProvider,
			Body: m.Text, Direction: extension.DirectionInbound,
		},
		Counterparty: extension.Counterparty{
			DisplayName: "Chat Buyer", Direction: extension.DirectionInbound,
			ChannelIdentity: extension.ChannelIdentity{Provider: chatProvider, ChannelUserID: m.Account},
		},
		OccurredPrecision: m.Precision,
		Parts:             m.Files,
	}
	if m.Precision != extension.PrecisionUnknown {
		rec.Activity.OccurredAt = time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC)
	}
	if m.SubjectID != "" {
		rec.ChosenSubject = &extension.ChosenSubject{Kind: m.SubjectKind, ID: m.SubjectID}
	}
	return rec
}

// ingestFiled is the unit's job. It reads what members filed and each member's
// own position, ingests their messages as one member-owned batch, and forgets
// what it filed. It answers every result so a test can read them.
func ingestFiled(ctx context.Context, rt extension.Runtime) (map[string][]extension.Result, error) {
	var rows []filedMessage
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		found, err := tx.Query(ctx, `SELECT body FROM `+chatTable+` ORDER BY body`)
		if err != nil {
			return err
		}
		defer found.Close()
		for found.Next() {
			var body string
			var msg filedMessage
			if err := found.Scan(&body); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(body), &msg); err != nil {
				return err
			}
			rows = append(rows, msg)
		}
		return found.Err()
	}); err != nil {
		return nil, err
	}
	results := map[string][]extension.Result{}
	byMember := map[string][]extension.Record{}
	for _, msg := range rows {
		byMember[msg.Member] = append(byMember[msg.Member], msg.record())
	}
	for member, recs := range byMember {
		on := extension.UserID(member)
		saved, err := rt.MemberIngestPosition(ctx, on, chatSource0)
		if err != nil {
			return results, err
		}
		batch, err := rt.IngestMemberBatch(ctx, on, chatSource0, recs, extension.BatchPosition("after:"+recs[len(recs)-1].Key), saved.Version)
		results[member] = batch.Results
		if err != nil {
			return results, err
		}
	}
	return results, rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM `+chatTable)
		return err
	})
}

const chatSource0 extension.SourceName = chatSystem

type chatEnv struct {
	*ingressEnv
	second ids.UUID
}

func setupMemberChat(t *testing.T, posture extension.Posture) *chatEnv {
	t.Helper()
	e := setupExtRuntime(t)
	unitsqltest.ScratchTable(t, chatTable)
	decl := extension.JobDeclaration{
		Unit: chatUnit, Job: chatJob, Queue: "default", Cadence: time.Hour,
		DispatcherTimeout: 30 * time.Second, Timeout: 30 * time.Second, MaxAttempts: 2,
		Tier: extension.TierAutoExecute, RequestedScope: extension.ScopeRead,
	}
	if err := RegisterExtensions([]extension.Extension{{
		Name: chatUnit, Version: "1.0.0", Description: "A neutral member-owned chat unit for tests.",
		Ingress: []extension.IngressSource{{
			System: chatSystem, Lands: []extension.RecordKind{extension.KindActivity}, PinnedPosture: posture,
		}},
		Channels: []extension.Channel{{Provider: chatProvider, CredentialModel: extension.CredentialPerMember}},
		Secrets:  []extension.SecretsRequest{{Key: ingressTestSecretKey, Scope: extension.SecretScopeUser}},
		Jobs: []extension.Job{{Name: chatJob, Handle: func(ctx context.Context, rt extension.Runtime) error {
			_, err := ingestFiled(ctx, rt)
			return err
		}}},
	}}, nil, []extension.JobDeclaration{decl}); err != nil {
		t.Fatalf("registering the unit: %v", err)
	}
	t.Cleanup(func() {
		setComposedJobs(nil)
		if err := jobs.RegisterComposed(nil); err != nil {
			t.Errorf("restoring the composed job table: %v", err)
		}
	})
	bindCaptureForTest(t, e)
	env := &chatEnv{ingressEnv: &ingressEnv{extRuntimeEnv: e, member: e.Rep1}, second: e.Rep2}
	registerProbeTransport(t, env.ingressEnv)
	for _, member := range []ids.UUID{e.Rep1, e.Rep2} {
		grantCapture(t, e, member)
		store := extsecrets.For(chatUnit, e.Pool, e.vault)
		if err := store.PutUser(e.callCtx(e.WS), extension.UserID(member.String()), ingressTestSecretKey, []byte("pat")); err != nil {
			t.Fatalf("depositing %s's credential: %v", member, err)
		}
	}
	return env
}

func (e *chatEnv) job() *callRuntime {
	return jobRuntimeFor(e.callCtx(e.WS), chatUnit, "1.0.0", "job/ingest", boundExtensionRuntime())
}

// runJob runs the unit's job tick and answers what it ingested.
func (e *chatEnv) runJob(t *testing.T) map[string][]extension.Result {
	t.Helper()
	results, err := ingestFiled(context.Background(), e.job())
	if err != nil {
		t.Fatalf("the unit's job: %v", err)
	}
	return results
}

// file stores a member's message through the route and runs the tick.
func (e *chatEnv) file(t *testing.T, msg filedMessage) extension.Result {
	t.Helper()
	rt, ctx := e.runtime(chatUnit)
	if err := fileByHand(ctx, rt, msg); err != nil {
		t.Fatalf("the unit's route: %v", err)
	}
	results := e.runJob(t)[msg.Member]
	if len(results) != 1 {
		t.Fatalf("the job answered %d results, want 1", len(results))
	}
	return results[0]
}

func (e *chatEnv) message(key string) filedMessage {
	return filedMessage{Member: e.member.String(), Key: key, Account: "acct-" + key, Text: "hello " + key}
}

func (e *chatEnv) activityOf(t *testing.T, key string) (id ids.UUID, audience string, precision string) {
	t.Helper()
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, audience, occurred_precision FROM activity WHERE source = $1 AND source_id = $2`,
			sdkChatSource, key).Scan(&id, &audience, &precision)
	})
	return id, audience, precision
}

func (e *chatEnv) lead(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO lead (id, full_name, status, source, captured_by, owner_id) VALUES ($1, 'A Lead', 'new', 'manual', 'human:x', $2)`,
			id, e.member)
		return err
	}); err != nil {
		t.Fatalf("seeding a lead: %v", err)
	}
	return id
}

func (e *chatEnv) links(t *testing.T, activity ids.UUID, column string) int {
	t.Helper()
	return e.countAsWorkspace(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND `+column+` IS NOT NULL`, activity)
}

// A message filed on a visible contact is linked to it, held, and binds no identity.
func TestAHandFiledMessageIsFiledOnTheChosenContactAndBornHeldUnderTheSourcesPosture(t *testing.T) {
	e := setupMemberChat(t, extension.PostureClassified)
	contact := e.SeedContact(t, "Chosen Contact", &e.member)
	msg := e.message("m-1")
	msg.SubjectKind, msg.SubjectID = extension.SubjectContact, contact.String()

	result := e.file(t, msg)

	if result.Disposition != extension.DispositionAccepted {
		t.Fatalf("disposition = %q (%s), want accepted", result.Disposition, result.Reason)
	}
	id, audience, _ := e.activityOf(t, "m-1")
	if audience != "participants" {
		t.Errorf("audience = %q, want participants: the source pins classified, so the message is born held", audience)
	}
	if got := e.links(t, id, "contact_id"); got != 1 {
		t.Errorf("%d contact links, want the chosen contact", got)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND posture_at_import = 'classified'`, id, e.member); got != 1 {
		t.Errorf("%d classified import rows for the member, want 1", got)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM contact_channel_identity WHERE provider = $1`, chatProvider); got != 0 {
		t.Errorf("%d identities bound, want none: a chosen subject is never re-resolved by identity", got)
	}
}

// A subject the member cannot see answers as one that is not there. It stores
// nothing and does not stop the rest of the batch.
func TestAChosenSubjectTheMemberCannotSeeIsRefusedAsNotFound(t *testing.T) {
	e := setupMemberChat(t, extension.PostureClassified)
	revokeRoles(t, e.extRuntimeEnv, e.member)
	grantPolicy(t, e.member, "ingress-own", `{"objects":{"activity":{"read":true,"create":true,"update":true},
		"contact":{"read":true,"create":true,"update":true},"company":{"read":true,"create":true,"update":true}},"row_scope":"own"}`)
	hidden := e.SeedContact(t, "Hidden", &e.second)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE contact SET visibility = 'owner' WHERE id = $1`, hidden)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	refused := e.message("hidden")
	refused.SubjectKind, refused.SubjectID = extension.SubjectContact, hidden.String()
	missing := e.message("missing")
	missing.SubjectKind, missing.SubjectID = extension.SubjectContact, ids.NewV7().String()
	fine := e.message("fine")
	rt, ctx := e.runtime(chatUnit)
	for _, msg := range []filedMessage{refused, missing, fine} {
		if err := fileByHand(ctx, rt, msg); err != nil {
			t.Fatal(err)
		}
	}
	results := e.runJob(t)[e.member.String()]

	// The unit reads its rows by body, so the keys come back as fine, hidden, missing.
	if len(results) != 3 {
		t.Fatalf("%d results, want 3", len(results))
	}
	hiddenAnswer, missingAnswer := results[1], results[2]
	if hiddenAnswer.Disposition != extension.DispositionSubjectNotFound ||
		missingAnswer.Disposition != hiddenAnswer.Disposition || missingAnswer.Reason != hiddenAnswer.Reason {
		t.Errorf("a hidden subject answered %+v and a missing one %+v, want the same not-found answer", hiddenAnswer, missingAnswer)
	}
	if results[0].Disposition != extension.DispositionAccepted {
		t.Errorf("the record with no subject answered %q, want accepted: a refusal must not stop the batch", results[0].Disposition)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM activity WHERE source = $1 AND source_id IN ('hidden', 'missing')`, sdkChatSource); got != 0 {
		t.Errorf("%d activities stored for refused subjects, want none", got)
	}
}

func TestAChosenLeadReceivesTheMessage(t *testing.T) {
	e := setupMemberChat(t, extension.PostureHeld)
	grantPolicy(t, e.member, "ingress-lead", `{"objects":{"activity":{"read":true,"create":true,"update":true},
		"contact":{"read":true,"create":true,"update":true},"company":{"read":true,"create":true,"update":true},
		"lead":{"read":true}},"row_scope":"all"}`)
	lead := e.lead(t)
	msg := e.message("lead-1")
	msg.SubjectKind, msg.SubjectID = extension.SubjectLead, lead.String()

	if result := e.file(t, msg); result.Disposition != extension.DispositionAccepted {
		t.Fatalf("disposition = %q (%s), want accepted", result.Disposition, result.Reason)
	}
	id, audience, _ := e.activityOf(t, "lead-1")
	if got := e.links(t, id, "lead_id"); got != 1 {
		t.Errorf("%d lead links, want 1", got)
	}
	if audience != "participants" {
		t.Errorf("audience = %q, want participants under a pinned held posture", audience)
	}
}

// A day-only time keeps its precision, and an unknown one is dated on the day
// the core received it, as a day.
func TestAMessageKeepsHowPreciselItsTimeIsKnown(t *testing.T) {
	e := setupMemberChat(t, extension.PostureClassified)
	day := e.message("day")
	day.Precision = extension.PrecisionDay
	unknown := e.message("unknown")
	unknown.Precision = extension.PrecisionUnknown
	instant := e.message("instant")
	rt, ctx := e.runtime(chatUnit)
	for _, msg := range []filedMessage{day, unknown, instant} {
		if err := fileByHand(ctx, rt, msg); err != nil {
			t.Fatal(err)
		}
	}
	for key, results := range e.runJob(t) {
		for _, r := range results {
			if r.Disposition != extension.DispositionAccepted {
				t.Fatalf("%s: %q (%s)", key, r.Disposition, r.Reason)
			}
		}
	}
	for key, want := range map[string]string{"day": "day", "unknown": "day", "instant": "instant"} {
		if _, _, got := e.activityOf(t, key); got != want {
			t.Errorf("%s is stored at precision %q, want %q", key, got, want)
		}
	}
	var onReceiptDay bool
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT occurred_at::date = now()::date FROM activity WHERE source_id = 'unknown'`).Scan(&onReceiptDay)
	})
	if !onReceiptDay {
		t.Error("an unknown time was not dated on the day the core received it")
	}
}

// Two members of one source keep their own positions. A stale writer is
// refused, a partial batch moves nothing, and a restart reads what was saved.
func TestEachMemberOfASourceKeepsTheirOwnPositionAndAStaleWriterIsRefused(t *testing.T) {
	e := setupMemberChat(t, extension.PostureClassified)
	one, two := extension.UserID(e.member.String()), extension.UserID(e.second.String())
	rt := e.job()
	ctx := context.Background()

	first, err := rt.IngestMemberBatch(ctx, one, chatSource0, []extension.Record{e.message("a-1").record()}, "p1", 0)
	if err != nil {
		t.Fatalf("the first member's batch: %v", err)
	}
	if first.Position.Version != 1 {
		t.Fatalf("version after a first save = %d, want 1", first.Position.Version)
	}
	if _, err := rt.IngestMemberBatch(ctx, two, chatSource0, nil, "q1", 0); err != nil {
		t.Fatalf("the second member's first save: %v", err)
	}
	if got, err := rt.MemberIngestPosition(ctx, one, chatSource0); err != nil || got.Position != "p1" || got.Version != 1 {
		t.Fatalf("the first member reads %+v, %v; want p1 at version 1 whatever the second saved", got, err)
	}

	if _, err := rt.IngestMemberBatch(ctx, one, chatSource0, nil, "p2", 1); err != nil {
		t.Fatalf("the writer holding version 1: %v", err)
	}
	_, err = rt.IngestMemberBatch(ctx, one, chatSource0, nil, "stale", 1)
	if !errors.Is(err, extension.ErrPositionConflict) || !errors.Is(err, extension.ErrConflict) {
		t.Fatalf("a stale writer got %v, want ErrPositionConflict, which is also ErrConflict", err)
	}
	if _, err := rt.IngestMemberBatch(ctx, one, chatSource0, nil, "again", 0); !errors.Is(err, extension.ErrPositionConflict) {
		t.Fatalf("a writer expecting a first save over a saved position got %v, want a conflict", err)
	}

	partial := e.message("a-2").record()
	partial.Activity.ChannelProvider = "someone_elses"
	partial.Counterparty.ChannelIdentity.Provider = "someone_elses"
	_, err = rt.IngestMemberBatch(ctx, one, chatSource0, []extension.Record{partial}, "p3", 2)
	if err == nil {
		t.Fatal("a record naming a transport the unit does not supply did not end the batch")
	}

	for name, who := range map[string]extension.UserID{
		"a bad id": "not-an-id", "the nil id": "00000000-0000-0000-0000-000000000000",
	} {
		if _, err := rt.MemberIngestPosition(ctx, who, chatSource0); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("reading the position of %s = %v, want ErrInvalid: the nil id is where a stream-wide position lives", name, err)
		}
	}
	stranger := extension.UserID(ids.NewV7().String())
	if _, err := rt.IngestMemberBatch(ctx, stranger, chatSource0, nil, "p", 0); !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("a save for a member who deposited no credential = %v, want ErrForbidden", err)
	}

	restarted := e.job()
	got, err := restarted.MemberIngestPosition(ctx, one, chatSource0)
	if err != nil || got.Position != "p2" || got.Version != 2 {
		t.Fatalf("after a failed batch and a restart the member reads %+v, %v; want p2 at version 2", got, err)
	}
	if other, err := restarted.MemberIngestPosition(ctx, two, chatSource0); err != nil || other.Position != "q1" {
		t.Fatalf("the second member reads %+v, %v; want q1", other, err)
	}
	if _, err := restarted.IngestBatch(ctx, one, chatSource0, nil, "shared"); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("a stream-wide batch on a member-owned source got %v, want ErrInvalid", err)
	}
}

// A request that carries a caller never ingests, and a unit with no pin lands
// as before.
func TestAMemberOwnedSourceStillRefusesAnAttendedIngest(t *testing.T) {
	e := setupMemberChat(t, extension.PostureClassified)
	rt, ctx := e.runtime(chatUnit)

	_, err := rt.Ingest(ctx, extension.UserID(e.member.String()), e.message("att").record())
	if !errors.Is(err, extension.ErrAttendedIngest) {
		t.Errorf("Ingest on a caller's invocation = %v, want ErrAttendedIngest", err)
	}
	_, err = rt.IngestMemberBatch(ctx, extension.UserID(e.member.String()), chatSource0, nil, "p", 0)
	if !errors.Is(err, extension.ErrAttendedIngest) {
		t.Errorf("IngestMemberBatch on a caller's invocation = %v, want ErrAttendedIngest", err)
	}
}
