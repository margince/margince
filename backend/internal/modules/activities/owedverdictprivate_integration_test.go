// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// protectedReasons are the holds a human or a confidentiality rule chose. No
// model may read mail held for any of them, whatever the setting says.
var protectedReasons = []string{
	ReasonConfidentialMarker, ReasonCounterparty, ReasonManual, ReasonVerdict,
	ReasonMembersOnly, ReasonInternalOperational,
}

// privateWait is a waiting customer's mail that reached only the rep's
// mailbox, held for reason.
func (e *loadEnv) privateWait(t *testing.T, subject, reason string) ids.UUID {
	t.Helper()
	activity := e.waitingFromToRep(t, subject, "buyer@customer.test", e.buyer(t))
	e.privateTo(t, activity, e.rep, reason)
	return activity
}

// privateTo narrows message to the participants for reason, with seat's
// mailbox the one that delivered it. Capture records the hold on the import
// row and the audience derivation carries it onto the message. Only a human
// writes ReasonManual, and SetAudience refuses captured mail, so that one
// reason is stamped directly.
func (e *loadEnv) privateTo(t *testing.T, message, seat ids.UUID, reason string) {
	t.Helper()
	posture, status, why := importHoldFor(reason)
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status, verdict_reason)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (activity_id, user_id) DO UPDATE
		SET posture_at_import = EXCLUDED.posture_at_import, verdict_status = EXCLUDED.verdict_status,
		    verdict_reason = EXCLUDED.verdict_reason`, message, seat, posture, status, why)
	if reason == ReasonManual {
		e.exec(t, `UPDATE activity SET audience = 'participants', audience_reason = $2 WHERE id = $1`, message, reason)
		return
	}
	system := asClassifier(e)
	if err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
		return RecomputeAudienceTx(system, tx, ids.From[ids.ActivityKind](message))
	}); err != nil {
		t.Fatalf("deriving the audience: %v", err)
	}
	var got string
	if err := e.owner.QueryRow(context.Background(), `SELECT coalesce(audience_reason, '') FROM activity WHERE id = $1`, message).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != reason {
		t.Fatalf("the derivation held the message for %q, want %q", got, reason)
	}
}

// importHoldFor is what capture records on an import row to hold mail for
// reason: a verdict's status, or the mailbox posture with the hold's word.
func importHoldFor(reason string) (posture, status, why *string) {
	held, pending := "held", "pending"
	switch reason {
	case ReasonPendingVerdict:
		return nil, &pending, nil
	case ReasonVerdict:
		return nil, &held, nil
	case ReasonPosture, ReasonManual:
		return &held, nil, nil
	}
	return &held, nil, &reason
}

// privateMailRequestsOff turns the setting off until the test ends.
func (e *loadEnv) privateMailRequestsOff(t *testing.T) {
	t.Helper()
	e.exec(t, `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, PrivateMailRequests.Key())
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, PrivateMailRequests.Key()); err != nil {
			t.Errorf("restoring the private-mail setting: %v", err)
		}
	})
}

// readerWroteFirst puts the rep's own sent mail earlier on message's thread.
func (e *loadEnv) readerWroteFirst(t *testing.T, message ids.UUID) {
	t.Helper()
	fromCounterparty(t, e, message, "buyer@customer.test")
	sent := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key,
			counterparty_email, counterparty_outbound_attested, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Our offer', now() - interval '5 days', $2,
			'buyer@customer.test', true, 'seed', 'system')`, sent, threadKeyOf(t, e, message))
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'from', $2)`, sent, e.rep)
}

// ourSent is the id of our one sent message on thread.
func (e *loadEnv) ourSent(t *testing.T, thread string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT id FROM activity WHERE direction = 'outbound' AND thread_key = $1`, thread).Scan(&id); err != nil {
		t.Fatalf("reading our sent message: %v", err)
	}
	return id
}

// waitingAs is message's waiting row as reader sees it.
func (e *loadEnv) waitingAs(t *testing.T, reader context.Context, message ids.UUID) WaitingReply {
	t.Helper()
	rows, err := storeKnowing(e).WaitingReplies(reader, time.Now())
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == message {
			return row
		}
	}
	t.Fatal("the message is not waiting for this reader")
	return WaitingReply{}
}

// hiddenFor reads the hidden-backlog figures as the rep.
func (e *loadEnv) hiddenFor(t *testing.T) HiddenBacklog {
	t.Helper()
	got, err := storeKnowing(e).HiddenWaiting(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	return got
}

// Mail private only because mail sharing is off is offered to the model. Its
// verdict lands, and the request it finds files a task its assignee alone reads.
func TestRoutinePrivateMailIsJudgedAndFilesAPrivateTask(t *testing.T) {
	e := setupLoad(t)
	waiting := e.privateWait(t, "Can you confirm the price?", ReasonWorkspaceFloor)
	if !unjudged(t, e)[waiting] {
		t.Fatal("mail private for a routine reason is not offered to the model")
	}

	source := seedEmailRequest(t, e, "Please send the report", "commitment", OwedVerdictAsksUs)
	e.exec(t, `UPDATE activity SET owed_verdict = NULL, owed_verdict_at = NULL, owed_verdict_ruleset = NULL WHERE id = $1`, source)
	e.privateTo(t, source, e.rep, ReasonWorkspaceFloor)
	store := storeKnowing(e)
	applied, err := store.SetOwedVerdict(asClassifier(e), source, OwedVerdictAsksUs, "prompts-test", dbNow(t, e))
	if err != nil || !applied {
		t.Fatalf("a verdict on routine private mail was refused: applied=%v err=%v", applied, err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var task ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&task); err != nil {
		t.Fatalf("no request task from routine private mail: %v", err)
	}
	got, err := store.GetActivity(e.asSeat(e.rep), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Audience == nil || *got.Audience != crmcontracts.ActivityAudienceSelected || got.AssigneeId == nil || ids.UUID(*got.AssigneeId) != e.rep {
		t.Fatalf("the task from private mail is not the rep's alone: audience %v, assignee %v", got.Audience, got.AssigneeId)
	}

	// A colleague who was not on the mail reads neither the mail nor its task.
	colleague := e.asSeat(e.other)
	if e.waitingAt(colleague, t, time.Now())[waiting] {
		t.Error("a colleague not on the private mail sees it waiting")
	}
	theirs, err := store.GetActivity(colleague, ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if theirs.Subject != nil {
		t.Error("a colleague not on the private mail reads its request task")
	}
}

// Held mail never reaches a model: it is neither offered nor written. It is
// counted as unchecked for the reader who sees it waiting.
func TestDeliberatelyHeldMailIsNeverOfferedToTheModel(t *testing.T) {
	for _, reason := range protectedReasons {
		t.Run(reason, func(t *testing.T) {
			e := setupLoad(t)
			held := e.privateWait(t, "Contract terms", reason)
			if unjudged(t, e)[held] {
				t.Fatal("held mail was offered to the model")
			}
			applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), held, OwedVerdictAsksUs, "prompts-test", dbNow(t, e))
			if err != nil || applied {
				t.Fatalf("a verdict landed on held mail: applied=%v err=%v", applied, err)
			}
			if row := e.waitFor(t, held); !row.JudgeWithheld {
				t.Error("held mail on the queue does not say no model will judge it")
			}
			if got := e.hiddenFor(t).PrivateUnchecked; got != 1 {
				t.Errorf("private_unchecked = %d, want the one held message", got)
			}
		})
	}
}

// A verdict reached while mail was shared is not re-judged once it is held.
func TestHeldMailIsNotReJudgedUnderNewRules(t *testing.T) {
	e := setupLoad(t)
	held := e.waitingFromToRep(t, "Contract terms", "buyer@customer.test", e.buyer(t))
	if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), held, OwedVerdictAsksUs, "prompts-old", dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	e.privateTo(t, held, e.rep, ReasonConfidentialMarker)
	rows, _, err := storeKnowing(e).OwedRestale(asClassifier(e), rulesetNew, 100, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == held {
			t.Fatal("held mail was offered for re-judging")
		}
	}
}

// With the setting off, routine private mail is not judged either. The queue
// falls back on the reader having written first, and counts what went unchecked.
func TestWithTheSettingOffPrivateMailFallsBackAndIsCounted(t *testing.T) {
	e := setupLoad(t)
	e.privateMailRequestsOff(t)
	waiting := e.privateWait(t, "Re: our offer", ReasonWorkspaceFloor)
	e.readerWroteFirst(t, waiting)

	if unjudged(t, e)[waiting] {
		t.Fatal("private mail was offered to the model with the setting off")
	}
	row := e.waitFor(t, waiting)
	if !row.JudgeWithheld || !row.OwnerWroteOnThread {
		t.Fatalf("withheld=%v readerWrote=%v, want both: the reader wrote first on mail no model reads",
			row.JudgeWithheld, row.OwnerWroteOnThread)
	}
	if got := e.hiddenFor(t).PrivateUnchecked; got != 1 {
		t.Errorf("private_unchecked = %d, want 1", got)
	}
}

// The reader-wrote evidence is the reader's own: a colleague's earlier mail on
// the thread is not the rep having opened it.
func TestAColleaguesEarlierMailIsNotTheReaderWritingFirst(t *testing.T) {
	e := setupLoad(t)
	e.privateMailRequestsOff(t)
	waiting := e.privateWait(t, "Re: our offer", ReasonWorkspaceFloor)
	e.readerWroteFirst(t, waiting)
	e.exec(t, `UPDATE activity_participant SET user_id = $2 WHERE role = 'from' AND user_id = $1`, e.rep, e.other)

	if row := e.waitFor(t, waiting); !row.JudgeWithheld || row.OwnerWroteOnThread {
		t.Fatalf("withheld=%v readerWrote=%v, want withheld and not the reader's",
			row.JudgeWithheld, row.OwnerWroteOnThread)
	}
}

// Until the confidentiality verdict lands, nobody knows whether a thread is
// confidential, so mail on a thread a seat still waits on is not offered.
func TestRoutinePrivateMailWaitsForItsThreadVerdict(t *testing.T) {
	for status, offered := range map[string]bool{"": true, "pending": false, "cleared": true} {
		t.Run("thread verdict "+status, func(t *testing.T) {
			e := setupLoad(t)
			waiting := e.privateWait(t, "Contract terms", ReasonPosture)
			if status != "" {
				e.exec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
					threadKeyOf(t, e, waiting), e.rep, status)
			}
			if got := unjudged(t, e)[waiting]; got != offered {
				t.Fatalf("offered = %v, want %v", got, offered)
			}
		})
	}
}

// A restricted message reaches no model, private for a routine reason or not.
func TestARestrictedPrivateMessageIsNeitherOfferedNorJudged(t *testing.T) {
	e := setupLoad(t)
	mail := e.privateWait(t, "Contract terms", ReasonWorkspaceFloor)
	e.exec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Controller', 'Statutory hold')`, mail)
	e.exec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(), archived_at = now(),
		restricted_at = now(), restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years'
		WHERE id = $1`, mail)
	if unjudged(t, e)[mail] {
		t.Fatal("a restricted message was offered to the model")
	}
	applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), mail, OwedVerdictAsksUs, "prompts-test", dbNow(t, e))
	if err != nil || applied {
		t.Fatalf("a verdict landed on a restricted message: applied=%v err=%v", applied, err)
	}
}

// Mail read while routine and held by a human before the verdict is written
// keeps no verdict: the write asks the rule again.
func TestMailHeldDuringTheModelCallIsNotJudged(t *testing.T) {
	e := setupLoad(t)
	mail := e.privateWait(t, "Contract terms", ReasonWorkspaceFloor)
	if !unjudged(t, e)[mail] {
		t.Fatal("routine private mail was not offered")
	}
	readAt := dbNow(t, e)
	e.privateTo(t, mail, e.rep, ReasonManual)
	applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), mail, OwedVerdictAsksUs, "prompts-test", readAt)
	if err != nil || applied {
		t.Fatalf("a verdict landed on mail held since it was read: applied=%v err=%v", applied, err)
	}
}

// The fallback asks whether the seat owing the reply wrote first, not whoever
// is reading: a colleague on the same mail sees the owner's evidence.
func TestTheFallbackReadsTheOwnerNotTheViewer(t *testing.T) {
	for writer, wrote := range map[string]bool{"owner": true, "viewer": false, "owner, unreadable to the viewer": false} {
		t.Run(writer, func(t *testing.T) {
			e := setupLoad(t)
			e.privateMailRequestsOff(t)
			waiting := e.privateWait(t, "Re: our offer", ReasonWorkspaceFloor)
			e.privateTo(t, waiting, e.other, ReasonWorkspaceFloor)
			e.readerWroteFirst(t, waiting)
			switch writer {
			case "viewer":
				e.exec(t, `UPDATE activity_participant SET user_id = $2 WHERE role = 'from' AND user_id = $1`, e.rep, e.other)
			case "owner, unreadable to the viewer":
				e.privateTo(t, e.ourSent(t, threadKeyOf(t, e, waiting)), e.rep, ReasonConfidentialMarker)
			}
			row := e.waitingAs(t, e.asSeat(e.other), waiting)
			if row.OwnerID != e.rep || row.OwnerWroteOnThread != wrote {
				t.Fatalf("owner %v, ownerWrote = %v; want the rep and %v", row.OwnerID, row.OwnerWroteOnThread, wrote)
			}
		})
	}
}

// The rep's private reply settles a private request and closes its reminder.
// With the check of private mail off, the request is not offered at all.
func TestAPrivateRequestIsSettledByAPrivateReply(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run(fmt.Sprintf("private mail checked %v", on), func(t *testing.T) {
			e := setupLoad(t)
			if !on {
				e.privateMailRequestsOff(t)
			}
			request := seedEmailRequest(t, e, "Please send the report", "commitment", OwedVerdictAsksUs)
			e.privateTo(t, request, e.rep, ReasonWorkspaceFloor)
			replyTo(t, e, request, "Here it is.", requestInstant.Add(time.Hour))
			e.privateTo(t, e.ourSent(t, threadKeyOf(t, e, request)), e.rep, ReasonWorkspaceFloor)
			mintReminder(t, e, requestInstant.Add(2*time.Hour))
			candidate, offered := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]
			if offered != on {
				t.Fatalf("offered for settlement = %v, want %v", offered, on)
			}
			if !on {
				return
			}
			if candidate.TaskID == nil {
				t.Fatal("the private request carries no reminder to complete")
			}
			if err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{
				Request: candidate, Verdict: RequestSettled, Confidence: 0.9, DecidedBy: "test-model",
			}); err != nil {
				t.Fatal(err)
			}
			if _, _, done, _ := taskFor(t, e, request); !done {
				t.Fatal("the private request's reminder stayed open after its reply settled it")
			}
		})
	}
}

// Only a human decides which mail reaches a model: an agent's write of the
// setting is refused and changes nothing.
func TestAnAgentCannotTurnThePrivateMailCheckOn(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	e.privateMailRequestsOff(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	h := NewHandlers(db).WithSettings(configtest.StoredOver(t, e.pool, ConfigSettings()))
	agent, ok := principal.Actor(e.settingsAdmin(true))
	if !ok {
		t.Fatal("missing admin")
	}
	agent.Type, agent.ID = principal.PrincipalAgent, "agent:"+e.rep.String()
	ctx := principal.WithActor(e.settingsAdmin(true), agent)
	req := httptest.NewRequest(http.MethodPatch, "/v1/activities/follow-up-settings",
		strings.NewReader(`{"follow_up_after_days": 2, "private_mail_requests": true}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateFollowUpSettings(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("an agent's PATCH answered %d, want 403", rec.Code)
	}
	if on := privateMailCall(e.settingsAdmin(true), t, h, http.MethodGet, ""); on == nil || *on {
		t.Fatalf("the setting reads %v after the refused write, want still off", on)
	}
}

// A seat's hold on the thread lands before the audience recompute does, so
// routine private mail on a held thread is not offered either.
func TestRoutinePrivateMailOnAHeldThreadIsNotOffered(t *testing.T) {
	e := setupLoad(t)
	waiting := e.privateWait(t, "Contract terms", ReasonPendingVerdict)
	e.exec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, 'held')`,
		threadKeyOf(t, e, waiting), e.rep)
	if unjudged(t, e)[waiting] {
		t.Fatal("mail on a thread a seat holds was offered to the model")
	}
}

// A reminder out of private mail goes only to a seat that may read that mail.
func TestNoReminderForASeatThatMayNotReadThePrivateMail(t *testing.T) {
	e := setupLoad(t)
	source := e.privateWait(t, "Please send the report", ReasonWorkspaceFloor)
	system := asClassifier(e)
	err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
		mail, err := readActivity(system, tx, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
		if err != nil {
			return err
		}
		for seat, may := range map[ids.UUID]bool{e.rep: true, e.other: false} {
			holder, err := auth.SeatPrincipal(system, tx, seat)
			if err != nil {
				return err
			}
			_, err = emailRequestTask(system, tx, mail, holder, time.Now())
			if may && err != nil {
				t.Errorf("the mail's own seat may not hold its reminder: %v", err)
			}
			if !may && !errors.Is(err, errSeatMayNotHoldPrivateMail) {
				t.Errorf("a seat not on the private mail was given its reminder: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Held mail keeps no automatic reminder, even with a verdict reached while it
// was shared: the pass files only mail it may judge.
func TestHeldMailFilesNoAutomaticReminder(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Contract terms", "commitment", OwedVerdictAsksUs)
	e.privateTo(t, source, e.rep, ReasonConfidentialMarker)
	if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("held mail filed %d automatic reminders", count)
	}
}

// Our own earlier mail goes to the model as context, so the same rule reads it.
// A held message of ours is never the context of a verdict.
func TestOurEarlierMailIsContextOnlyWhenTheModelMayReadIt(t *testing.T) {
	for reason, carried := range map[string]bool{ReasonWorkspaceFloor: true, ReasonConfidentialMarker: false} {
		t.Run(reason, func(t *testing.T) {
			e := setupLoad(t)
			waiting := e.privateWait(t, "Re: the plan", ReasonWorkspaceFloor)
			fromCounterparty(t, e, waiting, "buyer@customer.test")
			e.ourReply(t, "The plan", "Tuesday at two?", threadKeyOf(t, e, waiting), 5)
			e.privateTo(t, e.ourSent(t, threadKeyOf(t, e, waiting)), e.rep, reason)
			if got := candidate(t, e, waiting).PriorOutbound != nil; got != carried {
				t.Fatalf("our earlier mail carried = %v, want %v", got, carried)
			}
		})
	}
}

// A human review also sends the mail to a model, so it reads by the same rule.
func TestReviewReadsRoutinePrivateMailAndNeverHeldMail(t *testing.T) {
	for reason, readable := range map[string]bool{ReasonWorkspaceFloor: true, ReasonConfidentialMarker: false} {
		t.Run(reason, func(t *testing.T) {
			e := setupLoad(t)
			mail := e.waitingFromToRep(t, "Monthly reporting", "buyer@customer.test", e.buyer(t))
			if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), mail, OwedVerdictInformsUs, "prompts-test", dbNow(t, e)); err != nil {
				t.Fatal(err)
			}
			e.privateTo(t, mail, e.rep, reason)
			reviewer := e.asSeat(e.rep)
			err := database.WithWorkspaceTx(reviewer, e.pool, func(tx pgx.Tx) error {
				_, err := OwedReviewTx(reviewer, tx, mail, 4000, 4000)
				return err
			})
			if readable && err != nil {
				t.Fatalf("review of routine private mail failed: %v", err)
			}
			if !readable && !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("review of held mail answered %v, want not found", err)
			}
		})
	}
}

// The setting is served and saved through the follow-up settings, and the
// owed-reply check follows it.
func TestAnAdminTurnsThePrivateMailCheckOff(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, PrivateMailRequests.Key()); err != nil {
			t.Errorf("restoring the private-mail setting: %v", err)
		}
	})
	waiting := e.privateWait(t, "Can you confirm the price?", ReasonWorkspaceFloor)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	h := NewHandlers(db).WithSettings(configtest.StoredOver(t, e.pool, ConfigSettings()))
	admin := e.settingsAdmin(true)

	if on := privateMailCall(admin, t, h, http.MethodGet, ""); on == nil || !*on {
		t.Fatalf("GET private_mail_requests = %v, want on by default", on)
	}
	if !unjudged(t, e)[waiting] {
		t.Fatal("the check does not read routine private mail by default")
	}
	if on := privateMailCall(admin, t, h, http.MethodPatch, `{"follow_up_after_days": 2, "private_mail_requests": false}`); on == nil || *on {
		t.Fatalf("PATCH off answered %v", on)
	}
	if on := privateMailCall(admin, t, h, http.MethodGet, ""); on == nil || *on {
		t.Fatalf("GET after PATCH off = %v", on)
	}
	if unjudged(t, e)[waiting] {
		t.Fatal("the check still offers private mail once switched off")
	}
}

// privateMailCall answers the private_mail_requests a follow-up settings call
// returns, failing on anything but 200.
func privateMailCall(ctx context.Context, t *testing.T, h Handlers, method, body string) *bool {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/activities/follow-up-settings", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if method == http.MethodGet {
		h.GetFollowUpSettings(rec, req)
	} else {
		h.UpdateFollowUpSettings(rec, req)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("%s follow-up settings = %d: %s", method, rec.Code, rec.Body.String())
	}
	var out crmcontracts.FollowUpSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the settings: %v", err)
	}
	return out.PrivateMailRequests
}
