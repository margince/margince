// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One pass's allowance of second opinions.
//
// capture_counterparty_verdict's twenty-minute wall was sized when a pass cost
// one model call per sender: verdictCatchUpCap senders, verdictCatchUpCap
// calls, plus the occasional re-ask of an answer below verdictConfidenceFloor,
// which was the exception.
//
// The asymmetric floor made the re-ask ordinary. A creating answer now needs
// verdictCreateFloor, so a `person` or `advisor` between the two floors is
// re-asked — and borderline creating answers are the common case on a mailbox
// full of first-time senders. A pass sized for 200 calls could make 400, run
// past its wall, and be retried whole by River, with review staging and the
// noise sweeps waiting behind it. Nothing is lost; the backlog simply drains
// slower than the numbers say, and nothing says so.
//
// The bound here is the remedy that needs no measurement. Lowering the cap or
// raising the wall are both numbers — by how much, to what — and neither has an
// answer without measuring a live pass on real hardware. Bounding the re-asks
// makes the cost bounded BY CONSTRUCTION: at most cap + budget calls, whatever
// fraction of answers land between the floors and however fast the model is. So
// the twenty minutes is exactly as valid as it was before the floor landed.

// verdictReAskShare is how much of a pass's sender budget may be spent a second
// time: one call in ten, so a re-ask stays the exception it was when the wall
// was sized rather than becoming a second pass hidden inside the first.
//
// A share rather than a count, so lowering verdictCatchUpCap lowers this with
// it — a cap and a budget that drifted apart would be a wall sized against
// neither.
const verdictReAskShare = 10

// reAskBudget is what one pass has left to spend on second opinions.
//
// Per pass and not per workspace-run, for verdictCatchUpCap's own reason: a
// shared counter would let one large backlog spend the allowance and leave
// every workspace after it unable to ask twice about anything.
type reAskBudget struct{ left int }

// reAskBudgetFor sizes the allowance against the senders this pass may drain.
//
// At least one however small the cap: a pass allowed five senders that could
// not ask twice about any of them would answer a different question from the
// same pass allowed five hundred, and the floor's re-ask is not a luxury of
// large passes.
func reAskBudgetFor(maxVerdicts int) *reAskBudget {
	return &reAskBudget{left: max(1, maxVerdicts/verdictReAskShare)}
}

// spend takes one second opinion, reporting false when the pass has none left.
func (b *reAskBudget) spend() bool {
	if b.left <= 0 {
		return false
	}
	b.left--
	return true
}
