// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The attempt ladders this package inserts with.
//
// River resolves MaxAttempts as the explicit InsertOpts first, the args type's
// own InsertOpts second, and its default of 25 last. api/jobs.yaml carries the
// number for the two owners that can apply one — a fan-out child, whose helper
// reads it off the spec, and an args-owned kind, whose InsertOpts is held equal
// to the declaration by TestArgsOwnedAttemptCapsMatchTheirDeclaration. It
// refuses the number from a caller-owned kind, because publishing a cap nothing
// applies is the declared-versus-actual drift that contract exists to remove.
//
// So a caller-owned kind's cap lives here, in Go, next to the insert that
// applies it. TestEveryInsertDeclaresAnAttemptCap is what keeps a new one from
// simply being forgotten.

// periodicPassMaxAttempts is the ladder a scheduled pass rides when nothing
// else owns one, and it is small for the reason workspaceSweepOpts states
// about a fan-out child: the pass's real retry cadence is its own next tick,
// so River's ladder is only there to ride out a transient blip inside one tick
// window. Three is the house number api/jobs.yaml publishes for a fan-out
// child on exactly that argument, and a collapsed pass that does the work in
// its own row is the same shape with the enumeration removed.
//
// Left unset it is River's default of 25 on attempt-to-the-fourth backoff,
// which reaches days — and because retryable is one of activeSweepStates, a
// backing-off row SUPPRESSES every tick until it discards. A pass that fails
// deterministically therefore stops running rather than retrying, which is the
// one failure this cap exists to remove; three attempts span seconds, inside
// even the thirty-second cadences.
const periodicPassMaxAttempts = 3

// The two ladders a one-shot insert rides, and the one it rides when its own
// row owns the outcome. A caller-owned kind declares no max_attempts —
// api/jobs.yaml refuses to publish a number nothing applies — so its cap is
// named here and the question a site answers is only WHICH of these three it
// is. Left unnamed a site gets River's 25, which is a ladder reaching days for
// work no reader chose it for.
const (
	// sweptJobMaxAttempts is for a kind something re-issues: a periodic pass
	// that re-nominates the same subject, a poller that heals what a signal
	// missed, a recovery scan that re-enqueues a stranded row. The re-issue IS
	// the retry, so River's ladder only has to ride out a blip inside one
	// window — and where the insert dedupes over activeSweepStates, a longer
	// one would suppress the very re-issue it is waiting for.
	sweptJobMaxAttempts = 3

	// oneOffJobMaxAttempts is for a kind nothing re-issues: a person pressed a
	// button, or a message arrived once. The ladder carries BOTH failure shapes
	// alone — a dependency down for a minute, worth the retries, and a
	// malformed input or a code defect, worth none — which is the headroom
	// vcardIngestMaxAttempts, scheduledSendMaxAttempts and
	// embedReindexMaxAttempts were each given for the same reason.
	oneOffJobMaxAttempts = 5

	// rowOwnedMaxAttempts is for a kind whose DOMAIN ROW records the outcome
	// and whose retry lives somewhere else — the kinds whose api/jobs.yaml
	// fault: block says the worker logs and returns nil. River is not their
	// retry mechanism at all, so a second attempt would re-run work the engine
	// already concluded rather than recover anything.
	rowOwnedMaxAttempts = 1
)
