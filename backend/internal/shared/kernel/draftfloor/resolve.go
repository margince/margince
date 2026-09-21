// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import (
	"context"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Sender answers who is writing. It is a READ of the acting principal's own
// identity and writes nothing, which is what lets a drafting service that
// guarantees zero writes depend on it.
//
// A principal with no human behind it — the system, a connector acting on
// nobody's authority — answers with two empty strings and no error. That is not
// a failure: DRAFT-AC-E-6 makes an unsigned draft the specified answer there.
type Sender interface {
	ActorIdentity(ctx context.Context) (name, email string, err error)
}

// Clock is the current time, injected rather than read, per the house pattern.
type Clock func() time.Time

// BaseLanguage answers the installation's own configured language.
//
// It is the tier BELOW the correspondence and above the hard default: a thread
// too short to detect is still being written by a team who told us which
// language they work in, and answering English to a German installation is a
// worse guess than the one they configured. Injected as a function because it
// lives in identity, and this package is reached by every drafting surface.
type BaseLanguage func(ctx context.Context) string

// Resolver assembles the envelope every drafting surface is handed.
//
// It is one type rather than a method on each service because the fallbacks are
// the interesting part and they must not differ by surface: a language that
// will not resolve becomes the default, and a sender that will not resolve
// becomes nobody. Two copies of that would eventually disagree about which
// failure is fatal, and the drafting screen would break on one surface only.
type Resolver struct {
	sender Sender
	base   BaseLanguage
	now    Clock
	logger *slog.Logger
}

// NewResolver builds a resolver on the real clock, with no sender bound.
func NewResolver() *Resolver { return &Resolver{now: time.Now} }

// WithSender binds the identity lookup. Without one, every draft is unsigned.
func (r *Resolver) WithSender(sender Sender) *Resolver {
	r.sender = sender
	return r
}

// WithBaseLanguage binds the installation's configured language, the tier
// between the correspondence's own text and the hard default.
func (r *Resolver) WithBaseLanguage(base BaseLanguage) *Resolver {
	r.base = base
	return r
}

// WithClock replaces the clock, so a test states the date its case is about.
func (r *Resolver) WithClock(now Clock) *Resolver {
	if now != nil {
		r.now = now
	}
	return r
}

// WithLogger binds the logger the degrade paths report through.
func (r *Resolver) WithLogger(logger *slog.Logger) *Resolver {
	r.logger = logger
	return r
}

// Now is the resolver's clock, for a caller that needs the same instant the
// envelope was stamped with.
func (r *Resolver) Now() time.Time {
	if r == nil || r.now == nil {
		return time.Now()
	}
	return r.now()
}

// Written is what a draft is being written INTO: the correspondence it answers,
// and whatever the message itself already records about its language.
//
// Separate fields rather than one blob because they are not equally good
// evidence. Stored is a fact somebody wrote down at capture; Body and Subject
// are text to read; and the subject is the weaker of the two, being a line
// contacts often leave in the sender's language on a reply they wrote in theirs.
type Written struct {
	// Stored is the language recorded on the message, empty when none is.
	// It outranks detection: the same message read twice must not answer two
	// languages because a quoted chain grew underneath it.
	Stored string
	// Body is the correspondence itself, quoted history included.
	Body string
	// Subject is the fallback text, read only when the body says nothing.
	Subject string
}

// Resolve assembles the envelope from what the draft is written into and where
// the conversation stands.
//
// Nothing here can fail the draft. An unresolvable language falls through the
// ladder to the default (DRAFT-AC-E-2), and an unresolvable sender leaves the
// draft unsigned (DRAFT-AC-E-6) — a drafting screen that errors because it
// could not work out a greeting is worse than a draft the rep edits.
//
// The ladder is stored, then the text, then the installation's own language,
// then English. The base-language tier is what answers a first message, which
// has no correspondence at all to read: its only text is the rep's typed
// intent, far too short to clear the detector's bar.
func (r *Resolver) Resolve(ctx context.Context, written Written, state convstate.State) Envelope {
	now := r.Now()
	name, email := r.actor(ctx)
	// The register is read from the WHOLE correspondence, quoted history
	// included: which register two contacts are on is a property of the
	// relationship, and a single reply may contain neither form while the
	// exchange behind it is unmistakably du.
	//
	// Language first, and passed IN rather than fixed up afterwards: the
	// envelope carries a register only for a German draft, so a language
	// corrected after construction would leave a German register on an English
	// envelope, or drop one the other way.
	return NewEnvelopeWithRegister(r.language(ctx, written),
		textlang.DetectRegister(written.Body), state, now, name, email)
}

// language walks the ladder, taking the first tier that names a language this
// product actually speaks.
//
// An unsupported stored or configured value is skipped rather than trusted: it
// would reach the prompt as an instruction to write in a language nothing else
// in the product can render.
func (r *Resolver) language(ctx context.Context, written Written) textlang.Lang {
	// The stored label is above the ladder rather than in it: it is a fact
	// somebody recorded, where every tier below is evidence being read now.
	if textlang.Known(written.Stored) {
		return textlang.Lang(written.Stored)
	}
	var base string
	if r != nil && r.base != nil {
		base = r.base(ctx)
	}
	// The same ladder the footer under a sent message walks. Shared, so a
	// draft and the footer beneath it cannot pick two languages for one mail.
	return textlang.FirstKnown([]string{written.Body, written.Subject}, base)
}

// actor names the acting human, or nobody.
//
// A lookup failure is reported and then degraded past, not swallowed: the
// caller gets an unsigned draft, and the reason it happened is in the log
// rather than only in the shape of the output.
func (r *Resolver) actor(ctx context.Context) (name, email string) {
	if r == nil || r.sender == nil {
		return "", ""
	}
	name, email, err := r.sender.ActorIdentity(ctx)
	if err != nil {
		r.log().WarnContext(ctx, "draft sender identity unavailable; drafting unsigned", "err", err)
		return "", ""
	}
	return name, email
}

// log is the resolver's logger, defaulting rather than being required at
// construction so a caller that only wants a draft is not made to supply one.
func (r *Resolver) log() *slog.Logger {
	if r == nil || r.logger == nil {
		return slog.Default()
	}
	return r.logger
}
