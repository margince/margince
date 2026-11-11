// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/pkg/extension"
)

// Fault renders a worker's failure as a fixed operator sentence and keeps
// the cause reachable through errors.Is.
//
// River persists err.Error() into river_job.errors verbatim. That column
// has no workspace, no RLS, and a retention River chooses — so whatever a
// worker returns is stored, fleet-visible, for as long as the ladder runs.
// A provider refusing a message routinely names the address it refused, so
// the raw cause is the one thing that may not travel this way. It goes to
// the log, where the audience and the retention are the operator's own.
//
// This is the comms faultReason posture (a fixed sentence chosen by what
// the cause IS, never the cause's own text) applied at the seam every
// worker shares rather than in one module.
//
// The cause may be READ to choose among those sentences: technicalfault.go
// inspects its SHAPE — type, sentinel, a status integer — and picks from a
// closed authored set, so an operator learns that a host did not resolve without
// the host's name travelling with it. What may never travel is the cause's TEXT.
func Fault(err error) error { return FaultContext(context.Background(), err) }

// FaultContext is Fault with the caller's context on the log line, so an
// unclassified failure carries the correlation id the rest of the trace uses.
//
// It knows no River kind, so it can honour NO composed failure class: a class
// is verified against the vocabulary registered for the kind that is failing,
// and a caller who cannot say which kind that is cannot have a class verified
// on their behalf. A composed worker calls FaultForKind instead. A classified
// error arriving here is not lost — it falls through to the core sentinel
// underneath it, exactly as it did before classes existed.
func FaultContext(ctx context.Context, err error) error {
	return faultFor(ctx, "", err)
}

// FaultForKind is FaultContext for a worker that knows the River kind it is
// failing under, which is the only way a COMPOSED class can be honoured.
//
// THE KIND IS WHAT MAKES THE CHECK POSSIBLE, and the check is what makes the
// write path obey the boot validation. extension.Failure is a plain constructor:
// it accepts any FailureClass value a unit builds, including one whose Sentence
// was formatted from the cause — which is the accident this whole seam exists to
// prevent, since a provider's prose routinely names the address it refused. The
// declared set is validated and collision-checked at boot; a value handed over at
// tick time has been through none of that. So the sentence is persisted only when
// it is, exactly, one this installation registered for this kind.
//
// An unregistered class degrades to the same unclassified substitute a bypassed
// fault gets, and the cause goes to the log. That is a real loss of detail for
// the operator and it is the right trade: the alternative is a fleet-visible,
// unscoped column holding text nobody reviewed.
func FaultForKind(ctx context.Context, kind string, err error) error {
	return faultFor(ctx, kind, err)
}

// faultFor is the one body both entry points share, so the two cannot drift into
// two different orderings of the same decisions.
func faultFor(ctx context.Context, kind string, err error) error {
	if err == nil {
		return nil
	}
	// River's own control returns pass through UNTOUCHED. A snooze
	// reschedules and a cancel deliberately stops; neither is a failure and
	// neither carries a cause to publish. They reach a worker's return
	// through helpers as often as directly (telegrampoll.go's
	// answerPollFailure returns river.JobSnooze on a provider throttle), so
	// this check cannot live at the call sites — it has to be here, or every
	// routine throttle logs as an unclassified failure and River's own
	// errors.As classification reads a substituted message.
	//
	// It is also checked BEFORE the vocabulary, so a cancel carrying a known
	// sentinel stays a cancel: stopping deliberately is not failing.
	var snooze *river.JobSnoozeError
	var cancel *river.JobCancelError
	if errors.As(err, &snooze) || errors.As(err, &cancel) {
		return err
	}
	// The UNIT'S OWN classification wins over the core vocabulary, and is checked
	// before it. A tick that wrapped a core sentinel in one of its declared classes
	// looked at the whole operation and said what it means for THIS unit — that a
	// provider is unreachable, say, rather than that some read found nothing — and
	// that is the more useful of two true statements. The sentinel stays reachable
	// through errors.Is underneath, so nothing that classifies on it downstream is
	// affected by the order.
	//
	// Only a class this installation REGISTERED for this kind is honoured — see
	// FaultForKind for why the write path checks rather than trusts.
	//
	// An unregistered one does not return here. It falls through to the core
	// vocabulary below, because a unit that wrapped a core sentinel in a class it
	// forgot to declare should still get the sentinel's own sentence rather than
	// nothing: an undeclared class must cost the failure the unit's detail, not a
	// classification it would have had anyway.
	if class, ok := extension.FailureClassOf(err); ok {
		if registered, found := registeredFailureClass(kind, class); found {
			if in, asked := extension.RescheduleAfter(err); asked {
				return rescheduleFor(ctx, kind, registered.Class, in, err)
			}
			// THE CAUSE STILL GOES TO THE LOG. Classifying a failure says what
			// KIND of thing went wrong; it does not say which host did not
			// resolve, and that detail is the diagnosis — the thing this seam
			// promises is reachable somewhere, just not in a fleet-visible
			// column. Returning here without logging would trade a vague screen
			// for a silent log, which is the same operator stuck one step later.
			slog.ErrorContext(ctx, "jobs: a worker failed", faultLogAttrs(ctx, kind, registered.Class, err)...)
			return &fault{sentence: registered.Sentence, cause: err}
		}
		slog.ErrorContext(ctx, "jobs: a worker returned a failure class this installation did not declare for this kind, so its sentence is not published",
			faultLogAttrs(ctx, kind, class.Class, err)...)
	}
	if known, ok := sentinelFaultFor(err); ok {
		// The stored sentence is fixed and names neither the read nor the record
		// nor the step, so the cause is logged here exactly as it is for the
		// classes above and below.
		slog.ErrorContext(ctx, "jobs: a worker failed", faultLogAttrs(ctx, kind, "", err)...)
		return &fault{sentence: known.sentence, cause: err}
	}
	// AFTER the sentinels, because a cause carrying one has already been named
	// in the product's own terms and that name is the more useful of two true
	// statements: "the record this job names no longer exists" tells an
	// operator what to do, "the provider answered with a server error" tells
	// them where it happened.
	//
	// The cause is READ here and never projected — the shape decides, the
	// sentence is authored (technicalfault.go). An operator whose job failed
	// because a host did not resolve now reads that, instead of being sent to
	// a log to find out.
	if technical, ok := technicalFaultFor(err); ok {
		slog.ErrorContext(ctx, "jobs: a worker failed", faultLogAttrs(ctx, kind, technical.class, err)...)
		return &fault{sentence: technical.sentence, cause: err}
	}
	// The SAME attributes as the two classified lines above. This is the branch
	// whose sentence tells an operator the diagnosis is in the process log, so it
	// is the one line that must be findable — and it was the one carrying neither
	// the kind nor anything else identifying the tick.
	slog.ErrorContext(ctx, "jobs: a worker failed with an unclassified cause", faultLogAttrs(ctx, kind, "", err)...)
	return &fault{sentence: unrecognised, cause: err}
}

// sentinelFaultFor answers the vocabulary entry a cause's sentinel names.
func sentinelFaultFor(err error) (sentinelFault, bool) {
	for _, known := range vocabulary {
		if errors.Is(err, known.sentinel) {
			return known, true
		}
	}
	return sentinelFault{}, false
}

// ClassFor answers the core class a failure is recorded under, or "" when the
// core vocabulary cannot name it.
//
// For a caller that stores the class instead of River's sentence: the column
// then holds a token from this closed set and never the cause's text. The
// order is faultFor's: a sentinel names a failure before its shape does.
func ClassFor(err error) string {
	if known, ok := sentinelFaultFor(err); ok {
		return known.class
	}
	if technical, ok := technicalFaultFor(err); ok {
		return technical.class
	}
	return ""
}

// The bounds a requested postponement is held to before it reaches the queue.
//
// A delay is a REQUEST from a unit, exactly as a class is, and this is the half
// of the seam that makes the request safe to honour.
//
// WHAT THE CEILING PROTECTS, said precisely, because the imprecise version is
// checkably false: a postponed row is `scheduled`, and `scheduled` IS counted —
// health.go's waitingStates and the queue-depth gauge both include it, so a row
// postponed for a year still shows up as one piece of waiting work. What it
// escapes is the AGE reading. Every "how long has this waited" measurement in
// this package filters `scheduled_at <= now()`, correctly, because nothing
// scheduled for the future is late — so a far-future delay produces a count
// nobody can age, forever, and "waiting: 1" is what a healthy idle tick looks
// like too. The ceiling keeps the delay inside the window where a stuck
// postponement eventually becomes measurable instead of permanently plausible.
//
// The floor carries two jobs. River PANICS on a negative duration, so a unit
// that computed one from a clock — a deadline already past, a subtraction that
// went the wrong way — would take the worker process down rather than fail a
// tick, and a bound is the only place that can be caught once for everybody.
// Zero is not a panic but is River's "run me immediately", which for a failing
// tick is a spin against a provider that is already refusing; a second is long
// enough not to be one and short enough that no unit meaning "as soon as
// possible" is meaningfully denied.
//
// A CALLER STAYING UNDER THE CEILING IS NOT LEFT TO PROSE. A unit declaring a
// cadence above this bound reconciles its delay against that cadence perfectly
// and is then clamped to less, polling a refusing provider harder during an
// outage than in health. backend/gates/pollcadenceparity_test.go reads this bound
// out of this file and holds every postponing unit under it.
const (
	minRescheduleDelay = time.Second
	maxRescheduleDelay = 15 * time.Minute
)

// rescheduleFor turns a unit's postponement request into the queue's own control
// return.
//
// WARN, not ERROR, and the level is the message. A postponed tick is not a
// failure: nothing was lost, nothing is owed an operator, and logging it at the
// level a dead job uses would put a routine outage in the same lane as work that
// needs a human. It is logged at all because a connector that has been
// postponing itself for a day is a fact somebody eventually needs, and it is the
// one fact a snooze leaves nowhere else — River records no attempt error for a
// snooze, so this line and the unit's own row are the whole trail.
func rescheduleFor(ctx context.Context, kind, class string, in time.Duration, err error) error {
	delay := min(max(in, minRescheduleDelay), maxRescheduleDelay)
	attrs := append(faultLogAttrs(ctx, kind, class, err), "retry_in", delay.String())
	// A CLAMPED REQUEST SAYS SO. The bounds exist to catch a unit that computed a
	// delay wrong, and a clamp that logged only its own result made that mistake
	// invisible: a unit asking for 72 hours and a unit asking for fifteen minutes
	// produced byte-identical output, so the one thing the bound was written to
	// notice left no trace anywhere. The request is carried only when it differs,
	// because a field that repeats retry_in on every healthy line is noise the
	// clamped case then hides in.
	if delay != in {
		attrs = append(attrs, "requested", in.String())
	}
	slog.WarnContext(ctx, "jobs: a worker postponed its own tick rather than failing", attrs...)
	return river.JobSnooze(delay)
}

// faultLogAttrs is what every fault log line carries, spelled once so the three
// branches cannot describe the same failure three different ways.
//
// THE CORRELATION ID IS THE HANDLER'S, and it is not attached here. Every
// serving role installs a correlation-aware default
// (httpserver.InstallProcessLogger), so attaching it by hand would have both
// halves stamp the same key and a JSON line carry correlation_id twice. One
// thing stamps it, for every package-level call in the tree.
//
// THE WORKSPACE STILL IS attached here, because nothing else knows it. The
// handler reads the correlation id off the context and only that; a job's
// workspace is this seam's to say.
//
// An absent value is OMITTED rather than logged empty. A dispatcher has no
// workspace and the two kindless entry points have no kind; an empty attr would
// assert a value the failure does not have, and a reader filtering on it would
// match every one of them.
func faultLogAttrs(ctx context.Context, kind, class string, err error) []any {
	attrs := make([]any, 0, 6)
	if kind != "" {
		attrs = append(attrs, "kind", kind)
	}
	if class != "" {
		attrs = append(attrs, "class", class)
	}
	if ws, ok := principal.WorkspaceID(ctx); ok {
		attrs = append(attrs, "workspace_id", ws.String())
	}
	return append(attrs, errorAttr(err))
}

// errorAttr renders the cause as a STRUCTURED GROUP rather than one formatted
// string, because this log line is the only place the cause is allowed to be
// detailed and a collector should not have to parse prose to use it.
//
// WHY THE DETAIL IS SAFE HERE and nowhere else: river_job.errors is unscoped and
// fleet-visible, which is the whole reason Fault substitutes a fixed sentence
// before the queue sees the error. The process log is the other side of that
// trade — its audience and its retention are the operator's own — so this is
// where the cause is supposed to be legible.
//
// THE TYPE CHAIN EARNS ITS PLACE, and it is not in the message. The branch that
// most needs this line is the unclassified one, whose stored sentence says only
// that the diagnosis is in the process log; what its reader needs is which error
// a unit returned, so it can be given a class. err.Error() answers what the
// provider said, the chain answers which code said it. A postponement records no
// attempt error at all, so this line is its entire trail.
//
// Outermost first, matching the order errors.As resolves in, and bounded by the
// wrapping depth an error actually has — a chain is walked with Unwrap and stops
// where the wrapping stops.
func errorAttr(err error) slog.Attr {
	return slog.Group("error",
		slog.String("message", err.Error()),
		slog.String("type", fmt.Sprintf("%T", err)),
		slog.Any("chain", causeTypes(err)),
	)
}

// causeTypes names the Go type of every layer of a wrapped error, outermost
// first, so an operator reading a fault line can find the code that produced it.
//
// It follows single-cause Unwrap only. An errors.Join tree has no single next
// layer, and a reader that flattened one would present siblings as a chain of
// causes — a claim about which failure produced which that the tree does not
// make. Such an error stops the walk at itself, where its own type and its
// message (which carries every joined cause's text) are already recorded.
func causeTypes(err error) []string {
	types := make([]string, 0, 4)
	for at := err; at != nil; at = errors.Unwrap(at) {
		types = append(types, fmt.Sprintf("%T", at))
	}
	return types
}

// VettedSentence reports whether s is a sentence Fault itself would have
// written — one of the vocabulary's, or the unclassified fallback.
//
// It exists because river_job.errors is fleet-visible with no RLS and no
// redaction path (see Fault), so a surface that shows a failure to a human
// asks this rather than trusting the column. A worker that bypassed Fault
// and returned its raw cause stored that cause here; the answer for it is
// false, and the reader substitutes its own fixed text. River writes into
// that column too — its rescuer's "Stuck job rescued by JobRescuer" is not
// a Fault sentence and is correctly refused.
//
// The comparison is EXACT, never a prefix or a contains: a worker whose raw
// cause merely embeds a vetted sentence would otherwise carry the rest of
// its text through on the strength of the part that matched.
//
// The vocabulary stays unexported: a caller asks whether one string is
// vetted, it does not get the list to render or to match against by hand.
func VettedSentence(s string) bool {
	if s == unrecognised {
		return true
	}
	for _, known := range vocabulary {
		if s == known.sentence {
			return true
		}
	}
	return false
}

// The three SUBSTITUTE sentences a reader falls back to when a stored failure
// does not classify. They live together, in this package, because they are one
// set with one property to keep: each says something no classified failure may
// ever claim to say, so a composed class declaring any of them is refused
// (refuseCoreCollision). Two of them used to live in the HTTP surface that
// renders them, where the refusal could not see them — and a rule enforced over
// part of a set is a rule with a hole in the shape of the rest.
//
// They are NOT vocabulary entries: no sentinel maps to them, no class token names
// them, and VettedFailure answers no class for them. That is the point. Each is
// what the product says when it has nothing to say.
var (
	// unrecognised is what an unclassified cause becomes on the wire. It says
	// where the diagnosis went, because an operator reading it in a job list
	// otherwise has no next step.
	unrecognised = systemtext.Literal("jobs.unclassified")

	// UnvettedFailureReason is what an unrecognised STORED error becomes.
	//
	// It does NOT promise the diagnosis is in the process log. River writes its
	// own strings into that column too, and the rescuer's ("Stuck job rescued by
	// JobRescuer") means the worker's process died mid-job — so for that case,
	// one of the most common to reach here, a log pointer would be an instruction
	// to go read something that was never written. It says what is known and
	// where to look, and no more.
	UnvettedFailureReason = systemtext.Literal("jobs.unvetted")

	// NoRecordedCause is what a row with no stored error at all becomes.
	//
	// It is NOT the unvetted substitute. A cancelled job that never ran records
	// no attempt error, and telling its operator the job "failed for a reason
	// this surface cannot vet" asserts a failure that did not happen and points
	// at a log line nobody wrote. Nothing recorded is a different fact from
	// something unreadable, and the two must not render alike.
	NoRecordedCause = systemtext.Literal("jobs.no_cause")
)

// substitutes is the set no declared class may claim, spelled once so a fourth
// one added above is covered without anybody remembering to add it here.
var substitutes = []string{unrecognised, UnvettedFailureReason, NoRecordedCause}

// fault carries the vetted sentence on the wire and the real cause
// underneath, so errors.Is still classifies while Error() stays fixed.
type fault struct {
	sentence string
	cause    error
}

func (f *fault) Error() string { return f.sentence }
func (f *fault) Unwrap() error { return f.cause }

// sentinelFault is one core vocabulary entry: the sentinel it classifies and
// what the product says about it.
type sentinelFault struct {
	sentinel error
	class    string
	sentence string
	remedy   string
}

// vocabulary maps the shared sentinel registry to operator sentences. Each
// says what went wrong AND what it means for the job — an operator reading
// a failure list needs to know whether to retry, wait, or fix something.
//
// Every entry carries the same TRIPLE an extension unit declares (faultclass.go):
// a class token, the sentence, and the remedy. The two halves are one vocabulary
// rendered by one surface, so they hold the same shape — an operator should not be
// able to tell from a failure list which tier classified the failure, and a
// reader should not need two code paths to render it.
//
// TestEverySentinelIsClassifiedForTheJobSurface derives the coverage obligation
// from apperrors itself: a sentinel added there without an entry here fails the
// gate rather than silently reporting as unclassifiable the first time a job
// returns it.
var vocabulary = []sentinelFault{
	{apperrors.ErrNotFound, "record_gone", systemtext.Literal("jobs.record_gone.sentence"), systemtext.Literal("jobs.record_gone.remedy")},
	{apperrors.ErrConflict, "write_conflict", systemtext.Literal("jobs.write_conflict.sentence"), systemtext.Literal("jobs.write_conflict.remedy")},
	{apperrors.ErrVersionSkew, "version_skew", systemtext.Literal("jobs.version_skew.sentence"), systemtext.Literal("jobs.version_skew.remedy")},
	{apperrors.ErrPermissionDenied, "principal_not_permitted", systemtext.Literal("jobs.principal_not_permitted.sentence"), systemtext.Literal("jobs.principal_not_permitted.remedy")},
	{apperrors.ErrInvalidArgument, "invalid_argument", systemtext.Literal("jobs.invalid_argument.sentence"), systemtext.Literal("jobs.invalid_argument.remedy")},
	{apperrors.ErrConsentNotGranted, "consent_missing", systemtext.Literal("jobs.consent_missing.sentence"), systemtext.Literal("jobs.consent_missing.remedy")},
	{apperrors.ErrBudgetExceeded, "budget_spent", systemtext.Literal("jobs.budget_spent.sentence"), systemtext.Literal("jobs.budget_spent.remedy")},
	{apperrors.ErrRequiresApproval, "staged_for_approval", systemtext.Literal("jobs.staged_for_approval.sentence"), systemtext.Literal("jobs.staged_for_approval.remedy")},
	{apperrors.ErrSeatTierInsufficient, "seat_tier_insufficient", systemtext.Literal("jobs.seat_tier_insufficient.sentence"), systemtext.Literal("jobs.seat_tier_insufficient.remedy")},
	{apperrors.ErrSeatLimitReached, "seat_limit_reached", systemtext.Literal("jobs.seat_limit_reached.sentence"), systemtext.Literal("jobs.seat_limit_reached.remedy")},
	{apperrors.ErrScopeExceeded, "scope_exceeded", systemtext.Literal("jobs.scope_exceeded.sentence"), systemtext.Literal("jobs.scope_exceeded.remedy")},
	{apperrors.ErrApprovalTokenInvalid, "approval_token_spent", systemtext.Literal("jobs.approval_token_spent.sentence"), systemtext.Literal("jobs.approval_token_spent.remedy")},
	{apperrors.ErrUnsupportedBySoR, "unsupported_by_sor", systemtext.Literal("jobs.unsupported_by_sor.sentence"), systemtext.Literal("jobs.unsupported_by_sor.remedy")},
	{apperrors.ErrBaseCurrencyLocked, "base_currency_locked", systemtext.Literal("jobs.base_currency_locked.sentence"), systemtext.Literal("jobs.base_currency_locked.remedy")},
	{apperrors.ErrProviderUnusable, "provider_unusable", systemtext.Literal("jobs.provider_unusable.sentence"), systemtext.Literal("jobs.provider_unusable.remedy")},
	{apperrors.ErrRetentionHold, "retention_hold", systemtext.Literal("jobs.retention_hold.sentence"), systemtext.Literal("jobs.retention_hold.remedy")},
}

// SubstituteInLanguage translates vetted operational wording while withholding raw failures.
func SubstituteInLanguage(original string, language textlang.Lang) string {
	switch original {
	case unrecognised:
		return systemtext.Copy("jobs.unclassified", language)
	case UnvettedFailureReason:
		return systemtext.Copy("jobs.unvetted", language)
	case NoRecordedCause:
		return systemtext.Copy("jobs.no_cause", language)
	default:
		return original
	}
}
