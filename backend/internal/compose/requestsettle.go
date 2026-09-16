// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Judging whether our own reply settled what a customer asked of us.
//
// owed_verdict beside this answers a question about ONE message — does it ask
// the recipient side for something — and is written once, from that message
// alone. That is right for what it says and insufficient for what a reader then
// sees: a request answered within the hour still reads as owed a week later,
// because nothing ever asks the second question.
//
// This is the second question, and it is a question about the THREAD. It is
// only askable once the workspace has written back, so the candidates are
// requests carrying a later outbound message and a conversation nobody has
// answered is never judged here at all.
//
// WHY A MODEL AND NOT A SQL PREDICATE. "Thanks, I will check" is a reply and
// discharges nothing; "Passt, bis zum 30ten" answers a scheduling ask
// completely. Nothing about the shape of the rows tells those apart — only the
// words do — and a predicate that settled on the existence of an outbound
// message would close every request the moment somebody acknowledged it, which
// is the behaviour the two acknowledgement tests in the activities module pin
// against. With no model configured this engine does not run and those tests
// describe the product exactly as they did before.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// settleBatchSize is how many threads one call judges. Four rather than the
	// owed pass's ten, because a candidate here is a CONVERSATION and not a
	// message: each carries several bodies, and ten of them would push one call
	// past the window the light tier reads well.
	settleBatchSize = 4
	// settleConfidenceFloor: below it the thread is re-asked solo, and below it
	// again the verdict is recorded as unsure — which leaves the request owed,
	// exactly as it was before this pass existed.
	settleConfidenceFloor = 0.7
	// settleCatchUpCap bounds one pass per workspace, for the reason the owed
	// pass states: a shared counter lets one large backlog starve every
	// workspace behind it.
	settleCatchUpCap = 200
	// settleThreadMessages is how much of one conversation reaches the prompt:
	// the request and the messages that followed it, oldest first. Eight rather
	// than the extractor's six because this window starts at a fixed anchor and
	// spends its budget forwards, where that one slides backwards through
	// history and can afford to be smaller.
	settleThreadMessages = 8
)

// settleCandidate is one answered request with the conversation behind it.
type settleCandidate struct {
	Request  activities.RepliedRequest
	Messages []threadMessage
}

// RequestSettler drives the settlement pass for one workspace at a time.
type RequestSettler struct {
	pool  *pgxpool.Pool
	store *activities.Store
	brain completer
	log   *slog.Logger
	// now bounds the candidate read at an instant, and is a field so a test can
	// pin it: the trigger set turns on which reply is newest as of this moment.
	now func() time.Time
}

// NewRequestSettler builds the engine over the pool and one model lane.
//
// The own-domain seam is bound for the reason the owed classifier binds it: the
// candidates are drawn through the waiting queue's own predicates, which
// exclude a colleague's message, and unbound the pass would judge internal mail.
func NewRequestSettler(pool *pgxpool.Pool, brain completer, now func() time.Time, log *slog.Logger) *RequestSettler {
	if now == nil {
		now = time.Now
	}
	db := InstallationDB(pool)
	return &RequestSettler{
		pool: pool,
		store: activities.NewStore(db).WithOwnDomains(
			ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		brain: brain,
		log:   log,
		now:   now,
	}
}

// RunWorkspace judges up to cap answered requests in the workspace bound in ctx.
//
// A budget stop ends the pass cleanly: what is judged is committed and the rest
// is simply still unjudged, which the next cycle reads again.
func (s *RequestSettler) RunWorkspace(ctx context.Context, maxVerdicts int) error {
	if s.brain == nil {
		return nil
	}
	if maxVerdicts <= 0 {
		maxVerdicts = settleCatchUpCap
	}
	judged := 0
	// Bounded by CALLS as well as by verdicts, for the reason the owed pass
	// gives: a batch where every verdict abstains makes no progress by the
	// verdict count and would re-ask the same threads forever.
	calls := 0
	maxCalls := maxVerdicts/settleBatchSize + 1
	for judged < maxVerdicts && calls < maxCalls {
		batch, err := s.candidates(ctx, settleBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		calls++
		n, err := s.judgeBatch(ctx, batch)
		judged += n
		if errors.Is(err, ai.ErrBudgetDeferred) {
			s.log.InfoContext(ctx, "request settle: budget exhausted, stopping the pass",
				"judged", judged)
			return nil
		}
		if err != nil {
			return fmt.Errorf("request settle: draining the backlog: %w", err)
		}
		if n == 0 {
			s.log.InfoContext(ctx, "request settle: batch made no progress, moving on")
			return nil
		}
	}
	return nil
}

// candidates reads answered requests and the conversation behind each.
//
// The window starts AT THE REQUEST, which is what the settlement question is
// about: everything before it is a different exchange, and paying a model to
// read years of history to judge one ask would spend the budget on text that
// cannot change the answer.
func (s *RequestSettler) candidates(ctx context.Context, limit int) ([]settleCandidate, error) {
	// ONE instant for the candidate read and the conversation read alike. Two
	// calls to now() would let a message land between them and reach the model
	// as evidence over a trigger set that never saw it.
	asOf := s.now()
	requests, err := s.store.RepliedRequests(ctx, asOf, limit)
	if err != nil {
		return nil, fmt.Errorf("request settle: reading answered requests: %w", err)
	}
	if len(requests) == 0 {
		return nil, nil
	}
	var out []settleCandidate
	err = InstallationDB(s.pool).Tx(ctx, func(tx pgx.Tx) error {
		for _, request := range requests {
			messages, err := requestConversation(ctx, tx, request.RequestID, asOf)
			if err != nil {
				return err
			}
			// A conversation the window cannot read — the thread key is gone, or
			// every message has been narrowed since — is not judged rather than
			// judged on nothing.
			if len(messages) < 2 {
				continue
			}
			out = append(out, settleCandidate{Request: request, Messages: messages})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// requestConversation reads the request and the messages that followed it.
//
// ITS OWN READ, and not signal extraction's window, which was the first attempt
// and was wrong in a way that would have been hard to see. That window takes the
// NEWEST messages of a thread — it walks a long history backwards, which is
// right for "what has this conversation ever stated" — so on a thread with six
// messages after the request, the request itself falls out of the window while
// this prompt goes on saying the first message is the request. The model would
// then read a later exchange, find it answered, and close an ask nobody had
// done. The two callers want opposite ends of the same thread, and one function
// cannot honestly serve both.
//
// So this anchors: the request, then the OLDEST messages after it, which is the
// exchange that answered it. A thread that keeps going past the cap is read up
// to the cap, and what it loses is the tail rather than the question.
//
// asOf bounds it above, the same instant the trigger set used. Without it a
// scheduled send — a message written but not yet delivered — would reach the
// model as though the customer had already read it, and the whole point of the
// bound in the candidate query is that such a message has settled nothing.
func requestConversation(ctx context.Context, tx pgx.Tx, requestID ids.UUID, asOf time.Time) ([]threadMessage, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, coalesce(direction, ''), coalesce(subject, ''),
		       coalesce(left(body, $1), ''), occurred_at
		  FROM activity
		 WHERE thread_key = (SELECT thread_key FROM activity WHERE id = $2)
		   AND thread_key IS NOT NULL
		   AND kind = (SELECT kind FROM activity WHERE id = $2)
		   AND channel_provider IS NOT DISTINCT FROM
		       (SELECT channel_provider FROM activity WHERE id = $2)
		   AND archived_at IS NULL
		   -- The audience, tested HERE and not only in the read that offered
		   -- this request: the candidate query runs in its own statement and a
		   -- thread can be narrowed between the two, which under read-committed
		   -- this statement is the first to see.
		   AND audience = 'workspace' AND restricted_at IS NULL
		   AND occurred_at <= $3
		   AND (occurred_at, id) >= ((SELECT occurred_at FROM activity WHERE id = $2), $2)
		 ORDER BY occurred_at, id
		 LIMIT $4`, extractBodyLimit, requestID, asOf, settleThreadMessages)
	if err != nil {
		return nil, fmt.Errorf("request settle: reading the conversation: %w", err)
	}
	defer rows.Close()
	var out []threadMessage
	for rows.Next() {
		var message threadMessage
		if err := rows.Scan(&message.ID, &message.Direction, &message.Subject,
			&message.Body, &message.At); err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

// judgeBatch judges one batch with ONE model call and commits each verdict.
//
// The per-call commit IS the checkpoint. A thread below the floor is re-asked
// on its own — which escalates the routing ladder by being its own structured
// call — and one still below it is recorded UNSURE rather than guessed: the
// request stays owed and the watermark advances, so the same conversation is
// not re-read until somebody writes on it again.
func (s *RequestSettler) judgeBatch(ctx context.Context, batch []settleCandidate) (int, error) {
	verdicts, err := s.ask(ctx, batch)
	if err != nil {
		return 0, err
	}
	byID := map[string]settleCandidate{}
	for _, c := range batch {
		byID[c.Request.RequestID.String()] = c
	}
	judged := 0
	var retry []settleCandidate
	for _, v := range verdicts.results {
		candidate, ok := byID[v.ID]
		if !ok {
			continue // the validator guarantees this; belt and braces
		}
		if v.Confidence < settleConfidenceFloor {
			retry = append(retry, candidate)
			continue
		}
		if err := s.commit(ctx, candidate, v, verdicts.judge); err != nil {
			return judged, err
		}
		judged++
	}
	for _, candidate := range retry {
		solo, err := s.ask(ctx, []settleCandidate{candidate})
		if err != nil {
			return judged, err
		}
		result := settleResult{ID: candidate.Request.RequestID.String(), Verdict: activities.RequestUnsure}
		if len(solo.results) == 1 && solo.results[0].Confidence >= settleConfidenceFloor {
			result = solo.results[0]
		}
		if err := s.commit(ctx, candidate, result, solo.judge); err != nil {
			return judged, err
		}
		judged++
	}
	return judged, nil
}

// commit applies one verdict through the store's single transaction.
func (s *RequestSettler) commit(ctx context.Context, candidate settleCandidate, v settleResult, judge string) error {
	in := activities.RequestSettlementInput{
		Request:    candidate.Request,
		Verdict:    v.Verdict,
		Remaining:  strings.TrimSpace(v.Remaining),
		DueAt:      settleDueDate(v.DueAt),
		Confidence: float64(v.Confidence),
		DecidedBy:  judge,
	}
	if err := s.store.SettleRequest(ctx, in); err != nil {
		return fmt.Errorf("request settle: recording the verdict: %w", err)
	}
	return nil
}

// settleDueDate reads the model's date, or nothing.
//
// A date the model invented is worse than no date: it puts a deadline on
// somebody's task that nobody agreed to. So an unparseable value is dropped
// rather than guessed at, and the task simply stays undated — which is what
// every machine-filed reminder is anyway.
func settleDueDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if at, err := time.Parse(layout, value); err == nil {
			return &at
		}
	}
	return nil
}

// ask makes one structured call for the given conversations.
func (s *RequestSettler) ask(ctx context.Context, batch []settleCandidate) (settleAnswer, error) {
	resp, err := ai.Ask(ctx, s.brain, settleRequest(batch), settleShapeValid(batch))
	if err != nil {
		return settleAnswer{}, err
	}
	var payload settlePayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		return settleAnswer{}, fmt.Errorf("request settle: unparseable model output: %w", err)
	}
	if msg := validateSettlePayload(payload, batch); msg != "" {
		return settleAnswer{}, fmt.Errorf("request settle: %s", msg)
	}
	return settleAnswer{results: payload.Results, judge: judgeOf(resp)}, nil
}
