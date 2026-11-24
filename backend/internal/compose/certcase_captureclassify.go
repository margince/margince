// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for capture_classify/classify.
//
// It certifies the shipped path rather than a description of it. It drives the
// engine's own pass over an in-memory backlog. The request comes from
// classifyRequest and the engine's validator reads the reply. The floor and the
// solo re-ask run as they ship. A case that rebuilt any of them would measure a copy.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// captureClassifyFixture is ONE batch, as the backlog read hands it to the
// engine: a list, because this site labels several messages in one call and the
// prompt's whole security question is what one of them can say about another.
type captureClassifyFixture []captureClassifyMessage

// captureClassifyMessage is one backlog row in exactly the fields the prompt
// consumes. It carries nothing the corpus asserts and no id — see Prepare for
// both.
type captureClassifyMessage struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	// Inbound says the counterparty wrote this one. The same call judges whether
	// an inbound message was a positive reply, and that half of the prompt is
	// unreachable from a fixture with no way to say a message came from outside.
	//
	// It defaults to FALSE, which is the safe direction for an omitted field: a
	// fixture that does not say is treated as our own outbound mail and asked no
	// reply question, rather than certified as a customer's answer by silence.
	Inbound bool `json:"inbound"`
	// Channel names the chat service of a chat message, and is empty on mail.
	Channel string `json:"channel,omitempty"`
}

// backlogRow is the message as the backlog read returns it.
func (m captureClassifyMessage) backlogRow() unlabeledMessage {
	row := unlabeledMessage{
		ID: ids.NewV7(), Kind: string(crmcontracts.ActivityKindEmail),
		Subject: m.Subject, Body: m.Body, Inbound: m.Inbound,
	}
	if m.Channel != "" {
		row.Kind, row.ChannelProvider = activities.KindMessage, m.Channel
	}
	return row
}

// captureClassifyCases serves the one site that labels captured mail for
// attention routing.
type captureClassifyCases struct{}

func (captureClassifyCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskCaptureClassify,
		Variant: "classify",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare turns one batch and the labels the scenario expects into a runnable
// case, MINTING an activity id per message rather than reading it from either.
//
// Production takes those ids from the backlog rows, which no model has ever
// seen, so the only way an answer can carry one is to have read it out of this
// call's prompt. A fixture that supplied them would put them in the hands of
// whoever authored the expected reply, and a model echoing back ids it was
// handed would then be indistinguishable from one that labeled the right
// messages — which is exactly the confusion validateClassifyPayload exists to
// prevent.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (captureClassifyCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var messages captureClassifyFixture
	if err := json.Unmarshal(fixture, &messages); err != nil {
		return nil, fmt.Errorf("capture_classify/classify: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableBatch(messages); err != nil {
		return nil, err
	}
	var want []classifyExpected
	if err := json.Unmarshal(expected, &want); err != nil || want == nil {
		return nil, fmt.Errorf("capture_classify/classify: the expected answer is not a list of labels: %s", expected)
	}
	if err := refuseUnreachableLabels(want, len(messages)); err != nil {
		return nil, err
	}
	batch := make([]unlabeledMessage, len(messages))
	for i, m := range messages {
		batch[i] = m.backlogRow()
	}
	return &captureClassifyCase{batch: batch, expected: want}, nil
}

// refuseUnreadableBatch names a batch the backlog read could never have
// returned. The read caps a call at classifyBatchSize rows and truncates every
// body to classifyBodyLimit, so a fixture beyond either bound would certify a
// prompt the product cannot send — and it is truncated in the read rather than
// in the builder, which is why the bound is checked here and not applied.
func refuseUnreadableBatch(messages captureClassifyFixture) error {
	if len(messages) == 0 {
		return errors.New("capture_classify/classify: the fixture supplies no message, so there is nothing to label")
	}
	if len(messages) > classifyBatchSize {
		return fmt.Errorf(
			"capture_classify/classify: the fixture carries %d messages, but one call labels at most %d",
			len(messages), classifyBatchSize)
	}
	for i, m := range messages {
		if n := utf8.RuneCountInString(m.Body); n > classifyBodyLimit {
			return fmt.Errorf(
				"capture_classify/classify: message %d carries a body of %d characters, but the backlog read truncates every body to %d",
				i+1, n, classifyBodyLimit)
		}
		if activities.ClassifyBody(m.Body, classifyBodyLimit) != strings.TrimSpace(m.Body) {
			return fmt.Errorf(
				"capture_classify/classify: message %d carries quoted mail, but the backlog read sends only the sender's current message",
				i+1)
		}
	}
	return nil
}

// refuseUnreachableLabels names an expectation the validator can never satisfy:
// a label outside the closed set is refused on every reply, and a count that is
// not one label per message asserts about a message this batch does not carry or
// leaves one unasserted. Each would measure nothing for as long as it stayed in
// the corpus. Naming it here costs a parse; finding it later costs a paid run.
func refuseUnreachableLabels(want []classifyExpected, messages int) error {
	if len(want) != messages {
		return fmt.Errorf(
			"capture_classify/classify: the scenario expects %d labels for %d messages, and this site answers one label per message",
			len(want), messages)
	}
	for i, expect := range want {
		if (expect.Label != "" || !expect.judgesReply()) && !classifyLabels[expect.Label] {
			return fmt.Errorf(
				"capture_classify/classify: the scenario expects %q for message %d, which is not commitment|meeting|other",
				expect.Label, i+1)
		}
		for _, verdict := range []string{expect.Reply, expect.NotReply} {
			if verdict != "" && !replyVerdicts[verdict] {
				return fmt.Errorf(
					"capture_classify/classify: the scenario expects reply %q for message %d, which is not positive|negative|neutral",
					verdict, i+1)
			}
		}
	}
	return nil
}

// classifyExpected is what a scenario expects of one message. A bare string
// is a label. An object may also name the reply verdict wanted, or one the
// reply must not be, and may then leave the label open.
type classifyExpected struct {
	Label    string `json:"label"`
	Reply    string `json:"reply"`
	NotReply string `json:"not_reply"`
}

func (e *classifyExpected) UnmarshalJSON(raw []byte) error {
	if err := json.Unmarshal(raw, &e.Label); err == nil {
		return nil
	}
	type fields classifyExpected
	return json.Unmarshal(raw, (*fields)(e))
}

func (e classifyExpected) judgesReply() bool { return e.Reply != "" || e.NotReply != "" }

// disagreement names how one answer differs from the expectation, "" when
// it does not.
func (e classifyExpected) disagreement(answered classifyResult) string {
	switch {
	case e.Label != "" && answered.Label != e.Label:
		return fmt.Sprintf("is labeled %q where the scenario expects %q", answered.Label, e.Label)
	case e.Reply != "" && answered.Reply != e.Reply:
		return fmt.Sprintf("has reply %q where the scenario expects %q", answered.Reply, e.Reply)
	case e.NotReply != "" && answered.Reply == e.NotReply:
		return fmt.Sprintf("has reply %q, which the scenario rules out", answered.Reply)
	}
	return ""
}

// captureClassifyCase is one batch ready to be labeled, closed over the minted
// ids and the label the scenario expects for each message.
type captureClassifyCase struct {
	batch    []unlabeledMessage
	expected []classifyExpected
}

// What the pass left a message as. A message the pass has not settled stays open.
const (
	classifySettledLabeled  = "labeled"
	classifySettledUnsure   = "unsure"
	classifySettledDeclined = "declined"
	classifySettledOpen     = "open"
)

// classifyOutcome is what the pass stored for one message.
type classifyOutcome struct {
	Settled string `json:"settled"`
	Label   string `json:"label,omitempty"`
	Reply   string `json:"reply,omitempty"`
}

// classifyRun is what a run records. It holds the replies the model gave, in the
// order the pass asked, and what the pass stored for each message in fixture order.
// An unreadable reply ends the pass and is kept apart, since it counts against the
// model.
type classifyRun struct {
	Replies  []string          `json:"replies"`
	Messages []classifyOutcome `json:"messages,omitempty"`
	Unusable string            `json:"unusable,omitempty"`
}

// classifyBacklog is the backlog the pass reads and the label store it writes,
// in memory. A row leaves the backlog when the pass labels or stamps it, as it
// does in the database.
type classifyBacklog struct {
	rows    []unlabeledMessage
	settled map[ids.UUID]classifyOutcome
}

func (b *classifyBacklog) UnlabeledCaptureMessages(_ context.Context, _ string, limit, _ int) ([]unlabeledMessage, error) {
	var out []unlabeledMessage
	for _, row := range b.rows {
		if _, done := b.settled[row.ID]; !done && len(out) < limit {
			out = append(out, row)
		}
	}
	return out, nil
}

func (b *classifyBacklog) settle(id ids.UUID, outcome classifyOutcome) (bool, error) {
	if !slices.ContainsFunc(b.rows, func(row unlabeledMessage) bool { return row.ID == id }) {
		return false, nil
	}
	b.settled[id] = outcome
	return true, nil
}

func (b *classifyBacklog) SetCaptureLabel(_ context.Context, id ids.UUID, label string, _ int64) (bool, error) {
	return b.settle(id, classifyOutcome{Settled: classifySettledLabeled, Label: label})
}

func (b *classifyBacklog) SetCaptureLabelAndReply(_ context.Context, id ids.UUID, label string, _ int64, verdict, _ string) (bool, error) {
	return b.settle(id, classifyOutcome{Settled: classifySettledLabeled, Label: label, Reply: verdict})
}

func (b *classifyBacklog) MarkCaptureLabelDeclined(_ context.Context, id ids.UUID, _ string) (bool, error) {
	return b.settle(id, classifyOutcome{Settled: classifySettledDeclined})
}

func (b *classifyBacklog) MarkCaptureLabelUnsure(_ context.Context, id ids.UUID, _ string, _ int64) (bool, error) {
	return b.settle(id, classifyOutcome{Settled: classifySettledUnsure})
}

// outcomes is what the pass stored for each message, in fixture order.
func (b *classifyBacklog) outcomes() []classifyOutcome {
	out := make([]classifyOutcome, len(b.rows))
	for i, row := range b.rows {
		out[i] = classifyOutcome{Settled: classifySettledOpen}
		if settled, ok := b.settled[row.ID]; ok {
			out[i] = settled
		}
	}
	return out
}

// classifyRecorder is the brain the pass asks through. It records every request
// and reply. It does not implement the shape-retry seam, so each call goes out bare.
// A case that retried would certify the answer a model gives after being told to
// try again.
type classifyRecorder struct {
	completer aitasks.Completer
	requests  []model.Request
	replies   []string
	// failed is the completer's own failure. It stays apart from an answer the pass
	// refused, because a call that never completed is the lane's problem and no
	// measurement of the reply.
	failed error
}

func (r *classifyRecorder) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	r.requests = append(r.requests, req)
	resp, err := r.completer.Complete(ctx, req)
	if err != nil {
		r.failed = err
		return model.Response{}, err
	}
	r.replies = append(r.replies, resp.Text)
	return resp, nil
}

// Run drives the production pass over an in-memory backlog of this batch. The
// batch call, the confidence floor, the solo re-ask and the stamp of a message the
// models decline all happen as in production. The cap is the product default, which
// is what pays for those re-asks. The calls go bare, through classifyRecorder.
func (c *captureClassifyCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	brain := &classifyRecorder{completer: completer}
	backlog := &classifyBacklog{rows: c.batch, settled: map[ids.UUID]classifyOutcome{}}
	pass := &CaptureClassifier{store: backlog, brain: brain, log: slog.New(slog.DiscardHandler)}
	err := pass.RunWorkspace(ctx, capture.DefaultClassifyPassCap)
	trace := aitasks.Trace{Requests: brain.requests}
	run := classifyRun{Replies: brain.replies, Messages: backlog.outcomes()}
	var unusable *unusableClassifyAnswer
	switch {
	case errors.As(err, &unusable):
		run.Unusable = unusable.Error()
	case err != nil:
		return trace, fmt.Errorf("capture_classify/classify: %w", err)
	}
	output, err := json.Marshal(run)
	if err != nil {
		return trace, fmt.Errorf("capture_classify/classify: recording the run: %w", err)
	}
	trace.Output = string(output)
	return trace, nil
}

// Evaluate asks whether the pass left each message with the expected label. A
// reply the pass could not read is invalid. The rest is wrong by what the pass
// stored. A message left unsure or declined is one the pass would not label,
// whatever the model said first. The floor counts, as part of what ships.
func (c *captureClassifyCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var run classifyRun
	if err := json.Unmarshal([]byte(trace.Output), &run); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("the run was not recorded: %v", err)}
	}
	if run.Unusable != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: run.Unusable}
	}
	if len(run.Messages) != len(c.batch) {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("the run recorded %d messages for a batch of %d", len(run.Messages), len(c.batch)),
		}
	}
	if disagreements := c.disagreements(run.Messages); len(disagreements) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(disagreements, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// disagreements names every message the pass stored otherwise than the scenario
// expects, in fixture order. All of them, not the first: a run that labeled one
// message right and two wrong is not the near miss one line would read as.
//
// Messages are named by their position in the fixture, which is where their
// expectation was written. The minted id would name nothing a scenario author
// could look up, and the subject is captured text that has no business being
// echoed into a record.
func (c *captureClassifyCase) disagreements(stored []classifyOutcome) []string {
	var out []string
	for i, outcome := range stored {
		expect := c.expected[i]
		if outcome.Settled != classifySettledLabeled && expect.Label != "" {
			out = append(out, fmt.Sprintf("message %d was left %s, with no label, where the scenario expects %q", i+1, outcome.Settled, expect.Label))
			continue
		}
		if differs := expect.disagreement(classifyResult{Label: outcome.Label, Reply: outcome.Reply}); differs != "" {
			out = append(out, fmt.Sprintf("message %d %s", i+1, differs))
		}
	}
	return out
}
