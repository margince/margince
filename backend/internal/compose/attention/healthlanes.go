// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The HEALTH lanes' seams: the things that went wrong rather than the work
// waiting to be done.
//
// They share a shape the work lanes do not. Each is OPTIONAL — an installation
// that composes no producer for one gets an ABSENT lane, which says something
// different from an empty one — and each is per-READER rather than per-
// workspace, so a producer that cannot name a human refuses and the lane is
// rendered as withheld. renderhealthlanes.go draws the cards they produce.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SyncHealth is the overlay sync's current concerns: the poller backing off,
// the incumbent budget degraded, mirror classes stale or still backfilling,
// and the classes an incumbent-driven write overwrote here.
// Aggregated by the owning module (one concern per condition, never one per
// row), so a broken connector is one card and not a flood.
//
// The seam behind it answers apperrors.ErrModeNotOverlay for a workspace that
// never connected an incumbent, and the feed renders that as an ABSENT lane:
// an installation not in overlay mode does not look here, which is a
// different fact from a healthy sync and from a withheld one.
type SyncHealth interface {
	Concerns(ctx context.Context) ([]SyncConcern, error)
}

// SyncConcern is one sync condition worth the reader's glance. Kind names it;
// the other fields carry that kind's facts and are zero for the rest.
type SyncConcern struct {
	Kind string
	// ErrorClass and Failures describe a failing sweep; NextSweepAt is when
	// the poller retries.
	ErrorClass  string
	Failures    int
	NextSweepAt *time.Time
	// Band is the budget band a degraded budget sits in (warn or shed).
	Band string
	// Objects are the canonical classes a stale/backfilling concern covers.
	Objects []string
}

// CaptureHealth is the reader's own capture connections needing the reader's
// hand — re-authentication, a connection in error, a failing sync, a history
// import that ended in error. One concern per connection, its worst condition,
// derived by the capture module from the same view its settings screen lists.
//
// Capture is per-user, so the seam refuses a principal with no human behind
// it and the lane renders that refusal as withheld.
type CaptureHealth interface {
	CaptureConcerns(ctx context.Context) ([]CaptureConcern, error)
}

// CaptureConcern is one mailbox connection needing its owner.
type CaptureConcern struct {
	ConnectionID ids.UUID
	Kind         string
	Provider     string
	// AccountLabel is the display-only mailbox address when one was recorded.
	AccountLabel string
}

// AIWork is the reader's own AI runs that went wrong — failed inside the
// window the feed asks about, or live past the lease their source declared.
// Read from the AI-task projection, so it claims only AI work; the other
// health lanes carry what never reaches that projection.
//
// Per-user like CaptureHealth: the read refuses a principal with no human
// behind it, and the lane renders that refusal as withheld.
type AIWork interface {
	Troubled(ctx context.Context, since time.Time, limit int) ([]TroubledRun, error)
}

// TroubledRun is one AI run that went wrong for this reader.
type TroubledRun struct {
	ID ids.UUID
	// Kind is which task ran — the identity two failures of one broken task
	// share. The run's own summary is written per run and cannot group.
	Kind  string
	State string
	// Summary is the run's own recorded sentence, when it has one.
	Summary string
	// SubjectLabel is what the run was about, as its source named it.
	SubjectLabel string
	OccurredAt   time.Time
}

// Undelivered is the reader's own sends that were GIVEN UP ON — the ladder ran
// out, the mailbox would not transmit, the provider refused outright — read
// from the stamp the dispatcher's own park leaves.
//
// The distinction from Bounces is the whole point of having both: a bounce is
// mail that arrived somewhere and was refused, and this is mail that never
// left. From the sender's chair the two look the same — a thread that goes
// quiet — and today only one of them is ever reported.
//
// Per-user like the health lanes beside it: the read refuses a principal with
// no human behind it, and the lane renders that refusal as withheld.
type Undelivered interface {
	ParkedSends(ctx context.Context, since time.Time, limit int) ([]ParkedSend, error)
}

// ParkedSend is one send that was given up on.
type ParkedSend struct {
	ID ids.UUID
	// Subject is the send's own subject line — the name the reader knows it by.
	Subject string
	// Reason is the dispatcher's own words for giving up.
	Reason   string
	ParkedAt time.Time
	// PersonID is the person the send's activity is filed under, zero when it
	// is filed under none — the card then offers no open.
	PersonID ids.UUID
	// Recipient is the address that refused the send.
	//
	// Without it the card names a person and a subject, and a rep opening a
	// contact who carries three addresses cannot tell which one is dead — the
	// row reports a failure and leaves the reader to guess at the fix. Empty
	// when the send carries no recipient, and the card then says nothing about
	// where it was aimed rather than guessing.
	Recipient string
}

// Bounces is the reader's own sends whose delivery reports came back hard —
// read from the bounce stamp on the send's row, the same fact the timeline
// shows, rather than from a second reading of the capture stream.
//
// Per-user like the health lanes beside it: the read refuses a principal with
// no human behind it, and the lane renders that refusal as withheld.
type Bounces interface {
	HardBounces(ctx context.Context, since time.Time, limit int) ([]BouncedSend, error)
}

// BouncedSend is one send that did not arrive.
type BouncedSend struct {
	ID ids.UUID
	// Subject is the send's own subject line — the name the reader knows it by.
	Subject string
	// Reason is the receiving side's own words for the refusal.
	Reason    string
	BouncedAt time.Time
	// PersonID is the person the send's activity is filed under, zero when it
	// is filed under none — the card then offers no open.
	PersonID ids.UUID
	// Recipient is the address that refused the send.
	//
	// Without it the card names a person and a subject, and a rep opening a
	// contact who carries three addresses cannot tell which one is dead — the
	// row reports a failure and leaves the reader to guess at the fix. Empty
	// when the send carries none, and the card then says nothing about where it
	// was aimed rather than guessing.
	Recipient string
}

// AutomationHealth is the automations whose recent firings failed or were
// blocked — a rule its owner believes is running and is not. One entry per
// firing, newest first, bounded; only automations, because an engine
// workflow that is not one has no rule screen to open.
//
// Role-withheld like DSRs: the read is refused for everyone the automation
// screens refuse, and the lane renders that refusal as withheld.
type AutomationHealth interface {
	TroubledRuns(ctx context.Context, since time.Time, limit int) ([]TroubledAutomationRun, error)
}

// TroubledAutomationRun is one firing that did not do its work.
type TroubledAutomationRun struct {
	ID ids.UUID
	// AutomationID is the RULE, not this firing of it: the identity two
	// failures of one broken rule share, and which a rename does not move.
	AutomationID ids.AutomationID
	Name         string
	// Outcome is the contract's failed/blocked vocabulary.
	Outcome string
	// Reason is the engine's own recorded why, when it left one.
	Reason     string
	OccurredAt time.Time
}
