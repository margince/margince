// The approval queue: everything list_approvals answered, as the app's decision
// rows, each with its own Approve and Reject. Proposals one act staged together
// can also be answered together. A page that has more after it says so and asks
// the assistant for the next one, because a view cannot page on its own.

import { askAssistant, canCallTools, declareActions } from "../actions";
import actions from "../actions.json";
import { badge } from "../badge";
import { el } from "../bridge";
import {
  decide,
  decisionCard,
  isOpen,
  type Outcome,
  outcomesOf,
  type Proposal,
  proposalOf,
} from "../decision";
import { hero, segments } from "../hero";
import { button, panel, panelBody, viewRenderer } from "../parts";
import { refusalPanel } from "../summary";
import {
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import "../view.css";

declareActions(actions["approval-queue"]);

const SIX_HOURS_MS = 6 * 3_600_000;

/** render draws the queue, keeping the reader's place across a redraw. */
export const render = viewRenderer(draw);

function draw(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
  call: ToolCall,
): void {
  root.replaceChildren();
  if (call.refusal !== null) {
    root.appendChild(refusalPanel("Could not read the queue", call.refusal));
    return;
  }
  const page = asRecord(data);
  const proposals = asList(page.approvals)
    .map(proposalOf)
    .filter((p): p is Proposal => p !== null);
  const now = Date.now();
  const open = proposals.filter((p) => isOpen(p, now));
  const outcomes = outcomesOf(root);
  const waiting = open.filter((p) => outcomes.get(p.id)?.kind !== "decided");
  const card = panel("Waiting for your decision", {
    level: "h1",
    action: badge(
      String(waiting.length),
      waiting.length > 0 ? "accent" : "success",
    ),
  });
  const body = panelBody();
  if (proposals.length === 0) {
    body.appendChild(el("p", "intro", "Nothing is waiting for a decision."));
    card.appendChild(body);
    root.appendChild(card);
    return;
  }
  body.appendChild(lead(open, waiting, now));
  const again = () => render(root, data, warnings, call);
  const list = el("ol", "decision-list");
  for (const group of byBundle(proposals)) {
    const item = el("li");
    const together = group.filter((p) => isOpen(p, now));
    if (together.length > 1 && canCallTools()) {
      item.appendChild(
        bundleLine(together, outcomes, () =>
          decide(
            {
              approve: true,
              proposals: together,
              bundle: together[0]?.bundleID,
            },
            outcomes,
            again,
          ),
        ),
      );
    }
    for (const p of group) {
      item.appendChild(
        decisionCard(p, {
          layout: "row",
          now,
          outcome: outcomes.get(p.id),
          onDecide: (approve) =>
            decide({ approve, proposals: [p] }, outcomes, again),
        }),
      );
    }
    list.appendChild(item);
  }
  body.appendChild(list);
  const cursor = asText(page.next_cursor);
  if (cursor !== "") body.appendChild(morePage(cursor));
  card.appendChild(body);
  root.appendChild(card);
}

/** The queue at a glance: what waits, what runs out soon, and what was
 *  answered here, which grows as the reader works down the list. */
function lead(open: Proposal[], waiting: Proposal[], now: number): HTMLElement {
  const soon = waiting.filter((p) => {
    const at = Date.parse(p.expiresAt);
    return !Number.isNaN(at) && at - now < SIX_HOURS_MS;
  }).length;
  const byAssistant = waiting.filter((p) => p.byAssistant).length;
  const sentence =
    waiting.length === 0
      ? "Everything on this page is answered."
      : soon > 0
        ? `${soon} ${soon === 1 ? "runs" : "run"} out within 6 hours. ${byAssistant} proposed by an assistant.`
        : `${byAssistant} proposed by an assistant. Nothing runs out within 6 hours.`;
  return hero({
    kicker: "Approvals",
    figure: String(waiting.length),
    caption: waiting.length === 1 ? "decision waiting" : "decisions waiting",
    sentence,
    picture: segments([
      {
        label: "Answered here",
        value: open.length - waiting.length,
        tone: "success",
      },
      { label: "Running out soon", value: soon, tone: "warning" },
      { label: "Waiting", value: waiting.length - soon, tone: "neutral" },
    ]),
  });
}

/** byBundle keeps the answer's order and puts the members of a bundle together,
 *  where its first member stood. */
function byBundle(proposals: Proposal[]): Proposal[][] {
  const groups: Proposal[][] = [];
  const bundles = new Map<string, Proposal[]>();
  for (const p of proposals) {
    if (p.bundleID === "") {
      groups.push([p]);
      continue;
    }
    const known = bundles.get(p.bundleID);
    if (known !== undefined) {
      known.push(p);
      continue;
    }
    const group = [p];
    bundles.set(p.bundleID, group);
    groups.push(group);
  }
  return groups;
}

function bundleLine(
  members: Proposal[],
  outcomes: Map<string, Outcome>,
  approveAll: () => void,
): HTMLElement {
  const line = el("div", "choices bundle-line");
  const busy = members.some((p) => outcomes.get(p.id)?.kind === "busy");
  const settled = members.every((p) => outcomes.get(p.id)?.kind === "decided");
  line.appendChild(
    el("p", "meta", `${members.length} changes were proposed together.`),
  );
  if (!settled) {
    line.appendChild(
      button(`Approve all ${members.length}`, "ghost", approveAll, busy),
    );
  }
  return line;
}

function morePage(cursor: string): HTMLElement {
  const line = el("div", "choices");
  line.append(
    el("p", "meta", "More are waiting than this page shows."),
    button("Show the next page", "ghost", () =>
      askAssistant(
        `Show me the next page of approvals waiting for me (cursor ${cursor}).`,
      ),
    ),
  );
  return line;
}
