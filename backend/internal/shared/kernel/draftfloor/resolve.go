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

// UserLanguage answers the language the calling rep reads the app in, or ""
// when they never chose one. It sits above the installation's language: the
// rep is the one who sends the draft, so their own language is the better
// guess for a contact who has never written.
type UserLanguage func(ctx context.Context) string

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
	user   UserLanguage
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

// WithUserLanguage binds the calling rep's own app language, the tier between
// the typed purpose and the installation's language.
func (r *Resolver) WithUserLanguage(user UserLanguage) *Resolver {
	r.user = user
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
	// Rewrite is the draft being rewritten, read after the correspondence and
	// before the purpose: "make it shorter" must not change its language.
	Rewrite string
	// Purpose is what the rep typed to ask for the draft. It is read only
	// when the correspondence says nothing: a contact's own language beats
	// the rep's, but a rep who typed English to a contact who never wrote
	// wants English back.
	Purpose string
}

// Resolve assembles the envelope from what the draft is written into and where
// the conversation stands.
//
// Nothing here can fail the draft. An unresolvable language falls through the
// ladder to the default (DRAFT-AC-E-2), and an unresolvable sender leaves the
// draft unsigned (DRAFT-AC-E-6) — a drafting screen that errors because it
// could not work out a greeting is worse than a draft the rep edits.
//
// The ladder is stored, then the correspondence, then the draft being
// rewritten, then the rep's typed purpose, then the rep's own app language,
// then the installation's language, then English. A purpose too short to clear
// the detector's bar falls to the rep's app language, which is why that tier
// sits above the installation's.
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
		register(written), state, now, name, email)
}

// evidence is the text a draft's language and register are read from, in the
// order both ladders trust it. One list, so a rewrite or a first message with
// no correspondence keeps the du or Sie of the text that chose its language.
func (w Written) evidence() []string {
	return []string{w.Body, w.Subject, w.Rewrite, w.Purpose}
}

// register is the first du or Sie the evidence shows.
func register(written Written) textlang.Register {
	for _, text := range written.evidence() {
		if found := textlang.DetectRegister(text); found != textlang.RegisterUnknown {
			return found
		}
	}
	return textlang.RegisterUnknown
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
	var user, base func(context.Context) string
	if r != nil {
		user, base = r.user, r.base
	}
	// The helper the footer under a sent message walks too, so the two cannot
	// disagree about how a text or a fallback is judged.
	return textlang.FirstKnown(written.evidence(), deferred(ctx, user), deferred(ctx, base))
}

// deferred reads a configured language only when FirstKnown reaches its tier.
func deferred(ctx context.Context, read func(context.Context) string) func() string {
	return func() string {
		if read == nil {
			return ""
		}
		return read(ctx)
	}
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
