// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// The bounded-backfill seam: what a connector must answer to import a window
// of history, and what it reports while doing so.
//
// Its own file because it is OPTIONAL. A connector that cannot enumerate a
// mailbox backward from a date is simply not a Backfiller, and the engine
// type-asserts and refuses honestly — so nothing in the mandatory surface next
// door should have to be read to understand it.

import (
	"context"
	"strings"
	"time"
)

// Backfiller is the OPTIONAL bounded-backfill seam (ADR-0063): a connector
// implements it when its provider can enumerate a mailbox backward from a
// date boundary. Like Watcher, it is separate from Connector so a provider
// without a date-bounded listing simply is not a Backfiller; the backfill
// engine type-asserts and refuses honestly. Backfill paging is disjoint from
// Sync's cursor by construction — incremental moves forward from the
// connect-time watermark while backfill pages backward on its own token, and
// the capture key makes any overlap a no-op.
type Backfiller interface {
	// EstimateBackfill returns the provider-side message count newer than
	// after — the scope shown before anything spends (the preview op's
	// number).
	EstimateBackfill(ctx context.Context, auth Auth, after time.Time) (BackfillEstimate, error)

	// BackfillPage pulls ONE bounded page of messages newer than after,
	// emitting each through the Sink. It performs provider I/O like Sync;
	// the engine persists cursor and counters from the returned result.
	BackfillPage(ctx context.Context, auth Auth, after time.Time, pageToken string, sink Sink) (BackfillPageResult, error)
}

// BackfillEstimate is how big a window is, AND what kind of number that is.
//
// The two providers answer differently and a surface showing both has to be
// able to say which it has: Graph returns an exact `$count`, while Gmail is
// counted by paging message ids under a cap, so a large mailbox yields a
// LOWER BOUND rather than a total.
//
// Floor travels with the number rather than being inferred, because nothing
// downstream can tell the two apart by looking. A floor presented as a count
// is the worse failure of the two: a reader has no way to know which kind of
// number they are being shown, and the one they are consenting to is the one
// that reads as precise.
type BackfillEstimate struct {
	// Messages is what the provider counted.
	Messages int
	// Floor says the window holds AT LEAST Messages, and how many more is
	// unknown. False means the count is the total.
	Floor bool
}

// BackfillPageResult is one page's outcome: the token for the next page
// ("" = the window is exhausted) and the page's tally.
type BackfillPageResult struct {
	NextToken string
	Scanned   int
	Captured  int
	Skipped   int
}

// BackfillProgress carries a page's tally WHILE the page runs, so the engine
// can show progress that moves per message instead of once per committed
// page. A page is a hundred messages and minutes of provider I/O; without
// this the activation view sits at zero for the whole first page and reads
// as a dead import.
//
// Optional on both sides. The engine installs a reporter with
// WithBackfillProgress; a connector that never calls it reports only the
// BackfillPageResult it already returned, and behaves exactly as before.
// What a reporter records is advisory and transient — the page's own commit
// remains the one authority on a run's counters.
type BackfillProgress interface {
	// Observed reports THIS page's tally so far — the same three counts the
	// page's result carries, so a caller reading them mid-page still finds
	// scanned - captured = skipped. The numbers are absolute since the page
	// began, never deltas: a reporter that misses a call is corrected by the
	// next one instead of drifting, and a retried page restates rather than
	// double-counts.
	Observed(ctx context.Context, scanned, captured, skipped int)
}

// backfillProgressKey is the private context key — unexported and typed, so
// the reporter is reachable only through the two helpers below, never by
// another package reaching into the context for it directly.
type backfillProgressKey struct{}

// WithBackfillProgress installs the reporter a running page reports into.
// The engine calls this for the page it is about to run; nothing else should.
func WithBackfillProgress(ctx context.Context, p BackfillProgress) context.Context {
	return context.WithValue(ctx, backfillProgressKey{}, p)
}

// BackfillReporter is the value a connector reports through. It wraps the
// installed reporter, if any, so an unreported page costs a branch instead of
// a nil check at every call site.
type BackfillReporter struct{ to BackfillProgress }

// Observed forwards the page's tally, or discards it when nothing is
// listening.
func (r BackfillReporter) Observed(ctx context.Context, scanned, captured, skipped int) {
	if r.to != nil {
		r.to.Observed(ctx, scanned, captured, skipped)
	}
}

// BackfillProgressFrom returns the reporter for the running page — usable
// whether or not one was installed. Absence is ordinary: incremental sync
// installs no reporter, and neither do a connector's own tests.
func BackfillProgressFrom(ctx context.Context) BackfillReporter {
	p, _ := ctx.Value(backfillProgressKey{}).(BackfillProgress)
	return BackfillReporter{to: p}
}

// Container qualifies one provider container for NormalizedRecord.Containers
// and for a capture exclusion's stored value. Both sides call it, because a
// rule and the record it is matched against have to agree on the spelling
// character for character — the match is an equality.
//
// The provider is folded and the container is not: the prefix is ours and
// fixed, and what follows is the provider's own token — a Graph folder id is
// base64url, where two distinct folders can differ only in case, and an IMAP
// mailbox name is case-sensitive except for INBOX.
func Container(provider, container string) string {
	return strings.ToLower(provider) + ":" + container
}
