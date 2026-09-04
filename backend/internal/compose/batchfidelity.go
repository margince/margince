// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The id contract every batched model call shares: one result per requested id,
// each id verbatim, none repeated and none missing.
//
// One spelling because the two sites that ask it — the capture label and the
// owed verdict — are asking the same question about the same failure. A batch
// reply that answers a message nobody sent, answers one twice, or drops one is
// UNUSABLE rather than wrong, and each of those shapes is what a talked-into
// model produces: the whole reason the prompt fences each message separately is
// that one sender must not be able to reach another's.
//
// What is NOT here is each site's own vocabulary. "Is this a label" and "is this
// a verdict" are different closed sets over different questions, and folding
// them into one parameterised check would make the error message name a
// vocabulary the reader has to go and look up.

import "fmt"

// batchAnswer is one result as this contract sees it: which message it answers.
type batchAnswer interface {
	answeredID() string
}

// checkBatchFidelity names the first id-contract violation, or "" when the
// results answer each requested message and no other.
//
// It checks the IDS and stops. Each site then walks the same results for its own
// vocabulary and its confidence range, in that order — which is the order the
// two sites already had, and the order matters: the message reaches the model's
// bounded repair attempt, so telling it "that is not a verdict" where it used to
// hear "that confidence is out of range" changes what it tries next.
func checkBatchFidelity[T batchAnswer](results []T, requestedIDs []string) string {
	seen := map[string]bool{}
	want := make(map[string]bool, len(requestedIDs))
	for _, id := range requestedIDs {
		want[id] = true
	}
	for _, r := range results {
		id := r.answeredID()
		// Every echoed token is MODEL output, and a sender who got the model to
		// obey can choose it — so it is bounded before it reaches an error string
		// that ends up in the operator's log and, on a retry, back in the prompt.
		if !want[id] {
			return fmt.Sprintf("result id %q was not requested", clampToken(id))
		}
		if seen[id] {
			return fmt.Sprintf("result id %q appears twice", clampToken(id))
		}
		seen[id] = true
	}
	for _, id := range requestedIDs {
		if !seen[id] {
			return fmt.Sprintf("requested id %q is missing from the results", id)
		}
	}
	return ""
}
