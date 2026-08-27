// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What is waiting for review, counted as the READER can see it.
//
// The two numbers here name records whose visibility is per-reader by design: a
// duplicate pair is shown only to someone who can open BOTH sides of it, and a
// staged proposal only to someone who could decide it. Counted workspace-wide
// they become an existence oracle — the same number for everyone, moving as
// records the reader cannot open are created and resolved.
//
// The rest of the digest's counts stay workspace-wide and are right to. What
// the pipeline captured overnight is shared work, and the section says so.
//
// The counts are not computed here. The dedupe queue's both-sides-visible rule
// lives in the people store and decidability is a per-row probe inside the
// approvals engine; a count re-derived in this module would be a second answer
// to each, drifting the moment either rule changed.

import (
	"context"
)

// DigestReviewSource answers what is waiting for one reader.
//
// ctx carries THAT reader as the acting principal — their live grants and row
// scope — exactly as DigestProjectsSource does, so the numbers name only what
// they could open themselves.
//
// It takes no transaction, which is the difference from the projects source
// beside it: both counts are answered by engines that open their own, and
// handing them this build's transaction would mean either re-implementing their
// rules here or threading a transaction through two modules that do not take
// one. The cost is that the counts are read a moment outside the build's
// snapshot, which for a nightly tally of what is waiting is not a difference
// anybody can observe.
type DigestReviewSource func(ctx context.Context) (DigestReviewCounts, error)

// DigestReviewCounts is the pair, as the reader's own authority answers them.
type DigestReviewCounts struct {
	DedupeOpen       int
	ApprovalsPending int
}

// WithDigestReview wires the per-reader review counts into every digest this
// registry builds.
//
// Unwired, the counts stay zero rather than falling back to a workspace-wide
// number. A zero says "nothing waiting for you", which is wrong but harmless;
// the raw count says "this many exist", which is the disclosure this seam is
// here to close, and a fallback that reinstates it on any install that forgot
// to wire the seam would defeat the whole change.
func (r *Registry) WithDigestReview(src DigestReviewSource) *Registry {
	r.digestReview = src
	return r
}
