// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package websearch

// Whether a result may be CITED (ADR-0081 §3). Discovery is open; fetching is
// gated, and the two are different questions.
//
// The fetch half used to live here and no longer does: it is
// webread.MayFetch, beside robots and the SSRF guard, because it is a decision
// about FETCHING rather than about searching. Housed in this port it was
// reachable only by a consumer of the Brave seam, and in practice by nobody —
// it was written, tested and called by nothing while the enrich tool handed a
// caller-supplied URL straight to the fetcher. It would also have been deleted
// with this port if the seam were ever retired, taking a guard with it.
//
// A LinkedIn URL is a perfectly good citation and a forbidden fetch — the
// profile page is where the claim lives, and saying so costs nobody anything.
// Conflating the two would throw away the metadata that makes this seam
// useful, which is why the split survives the move.

// Citable reports whether a result may be quoted as evidence. Every result
// is: the provider returned it from a public index, and a citation asserts
// only that the claim appears at that address on that date.
//
// It exists as a named function rather than an assumed true so a reader of
// the calling code sees the distinction from MayFetch stated rather than
// implied.
func Citable(Result) bool { return true }
