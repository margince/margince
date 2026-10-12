// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturemetrics

// The backfill half: what a run records travels on its context, so the sink,
// which serves incremental sync too, times its stages only while a backfill
// is the caller. Outside a run every observer here is a no-op.

import (
	"context"
	"sync"
	"time"
)

type runKey struct{}

type messageKey struct{}

// run names the provider one backfill job is paging. The job opens it before
// the provider is known, and the pager fills it in once the connection is read.
type run struct {
	mu       sync.Mutex
	provider string
}

func (r *run) providerName() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.provider
}

// WithRun opens a backfill run on ctx for the job that pages it, so the waits
// the job chooses are attributed to the provider the pager names.
func WithRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, runKey{}, &run{})
}

// ForProvider names the provider the run on ctx is paging, opening a run when
// ctx carries none.
func ForProvider(ctx context.Context, provider string) context.Context {
	if r := runOf(ctx); r != nil {
		r.mu.Lock()
		r.provider = provider
		r.mu.Unlock()
		return ctx
	}
	return context.WithValue(ctx, runKey{}, &run{provider: provider})
}

func runOf(ctx context.Context) *run {
	r, _ := ctx.Value(runKey{}).(*run)
	return r
}

// providerOf answers the provider of the run on ctx, or false outside a run
// and before the pager has named one.
func providerOf(ctx context.Context) (string, bool) {
	r := runOf(ctx)
	if r == nil {
		return "", false
	}
	provider := r.providerName()
	return provider, provider != ""
}

// ObserveStage records how long one message spent in stage.
func ObserveStage(ctx context.Context, stage string, elapsed time.Duration) {
	if provider, ok := providerOf(ctx); ok {
		shared.observeStage(provider, stage, elapsed)
	}
}

// ObservePage records one provider page by the class its error falls in.
func ObservePage(ctx context.Context, err error) {
	if provider, ok := providerOf(ctx); ok {
		shared.observePage(provider, pageResult(err))
	}
}

// ObserveDeferral records the wait a backfill chose after a page failed on
// cause, beside the Retry-After the provider asked for.
func ObserveDeferral(ctx context.Context, wait time.Duration, cause error) {
	if provider, ok := providerOf(ctx); ok {
		reason, asked := deferralOf(cause)
		shared.observeFaultWait(provider, reason, wait, asked)
	}
}

// ObservePacing records the yield a backfill takes between two good pages.
func ObservePacing(ctx context.Context, wait time.Duration) {
	if provider, ok := providerOf(ctx); ok {
		shared.observeSnooze(provider, reasonPacing, wait)
	}
}

// ObserveResumed records the yield a job takes to page a run that was reopened
// while the job was ending it.
func ObserveResumed(ctx context.Context, wait time.Duration) {
	if provider, ok := providerOf(ctx); ok {
		shared.observeSnooze(provider, reasonResumed, wait)
	}
}

// ObserveInPageWait records a rate-limit wait the page took inside itself,
// beside the Retry-After the provider asked for.
func ObserveInPageWait(ctx context.Context, wait, asked time.Duration) {
	if provider, ok := providerOf(ctx); ok {
		shared.observeFaultWait(provider, reasonInPage, wait, asked)
	}
}

// Message is one backfilled message's tally: the last funnel outcome the
// capture trace recorded for it while it was walked.
type Message struct {
	mu       sync.Mutex
	provider string
	outcome  string
}

// BeginMessage opens a message on ctx. Outside a run it answers a nil Message,
// whose End is a no-op.
func BeginMessage(ctx context.Context) (context.Context, *Message) {
	provider, ok := providerOf(ctx)
	if !ok {
		return ctx, nil
	}
	m := &Message{provider: provider}
	return context.WithValue(ctx, messageKey{}, m), m
}

// NoteOutcome records the trace outcome the pipeline decided for the message
// on ctx.
func NoteOutcome(ctx context.Context, outcome string) {
	m, _ := ctx.Value(messageKey{}).(*Message)
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outcome = outcome
}

// Refuse counts the message as one the capture refused and the page walked
// past.
func (m *Message) Refuse() {
	if m != nil {
		shared.observeMessage(m.provider, OutcomeRefused)
	}
}

// End counts the message once, under the outcome its walk came to: failed on
// an error that ended the page, the traced decision when there was one, and
// otherwise captured or skipped as the connector tallied it.
func (m *Message) End(captured bool, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	outcome := m.outcome
	m.mu.Unlock()
	switch {
	case err != nil:
		outcome = OutcomeFailed
	case outcome == "" && captured:
		outcome = OutcomeCaptured
	case outcome == "":
		outcome = OutcomeSkipped
	}
	shared.observeMessage(m.provider, outcome)
}
