// The approval card: a change an assistant proposed and a human has to release.
// It offers the two answers it can be given. It draws what read_approval returns.
// Approve and Reject call decide_approvals through the host. The same engine,
// authority checks and audit row then apply as when the decision is made in the
// inbox.

import { declareActions } from "../actions";
import actions from "../actions.json";
import { decide, decisionCard, outcomesOf, proposalOf } from "../decision";
import { viewRenderer } from "../parts";
import { refusalPanel } from "../summary";
import type { ToolCall, Warning } from "../types";
import "../view.css";

declareActions(actions.approval);

/** render draws the card, keeping the reader's place across a redraw. */
export const render = viewRenderer(draw);

function draw(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
  call: ToolCall,
): void {
  root.replaceChildren();
  const proposal = proposalOf(data);
  if (proposal === null) {
    root.appendChild(refusalPanel("No approval to show", call.refusal));
    return;
  }
  const outcomes = outcomesOf(root);
  const again = () => render(root, data, warnings, call);
  root.appendChild(
    decisionCard(proposal, {
      layout: "deck",
      now: Date.now(),
      outcome: outcomes.get(proposal.id),
      onDecide: (approve) =>
        decide({ approve, proposals: [proposal] }, outcomes, again),
    }),
  );
}
