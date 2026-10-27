// The approval card: a change an assistant proposed and a human has to release.
// It offers the two answers it can be given. It draws what read_approval returns.
// Approve and Reject call decide_approvals through the host. The same engine,
// authority checks and audit row then apply as when the decision is made in the
// inbox.
//
// Approving an assistant's own proposal releases it; the assistant still has to
// run the call again, which is why the settled line says so instead of claiming
// the change happened.

import {
  askAssistant,
  callServerTool,
  canCallTools,
  declareActions,
} from "../actions";
import actions from "../actions.json";
import { badge } from "../badge";
import { el } from "../bridge";
import { button, panel, panelBody, viewRenderer } from "../parts";
import { type Change, changesOf, changeTable, refusalPanel } from "../summary";
import {
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import "../view.css";

declareActions(actions.approval);

type Item = {
  id: string;
  kind: string;
  status: string;
  summary: string;
  proposedBy: string;
  expiresAt: number | null;
  changes: Change[];
  evidence: string[];
};

type Outcome =
  | { kind: "busy" }
  | { kind: "decided"; approved: boolean }
  | { kind: "failed"; reason: string; unknown?: true };

const decided = new WeakMap<HTMLElement, Map<string, Outcome>>();

function outcomesOf(root: HTMLElement): Map<string, Outcome> {
  let found = decided.get(root);
  if (found === undefined) {
    found = new Map();
    decided.set(root, found);
  }
  return found;
}

/** The words for each kind of proposer, by the prefix its id carries. */
const PROPOSERS: Readonly<Record<string, string>> = {
  agent: "an assistant",
  connector: "a connected app",
  human: "a colleague",
  system: "Margince",
};

/** proposerOf says who proposed it and for whom; an id names no one. */
function proposerOf(proposedBy: string, onBehalfOf: unknown): string {
  const kind = proposedBy.split(":")[0] ?? "";
  const who = PROPOSERS[kind] ?? "someone";
  const behalf = asRecord(onBehalfOf);
  if (behalf.is_you === true) return `${who} for you`;
  const name = asText(behalf.name);
  return name === "" ? who : `${who} for ${name}`;
}

function itemOf(raw: unknown): Item | null {
  const a = asRecord(raw);
  const id = asText(a.staged_action_id);
  if (id === "") return null;
  const expires = Date.parse(asText(a.expires_at));
  return {
    id,
    kind: asText(a.kind),
    status: asText(a.status),
    summary: asText(a.summary),
    proposedBy: proposerOf(asText(a.proposed_by), a.on_behalf_of),
    expiresAt: Number.isNaN(expires) ? null : expires,
    changes: changesOf(a.changes),
    evidence: asList(a.evidence)
      .map((e) => asText(asRecord(e).evidence_snippet))
      .filter((snippet) => snippet !== ""),
  };
}

/** render draws the card, keeping the reader's place across a redraw. */
export const render = viewRenderer(draw);

function draw(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
  call: ToolCall,
): void {
  root.replaceChildren();
  const item = itemOf(data);
  if (item === null) {
    root.appendChild(refusalPanel("No approval to show", call.refusal));
    return;
  }
  const outcomes = outcomesOf(root);
  const again = () => render(root, data, warnings, call);
  const card = panel("Waiting for a decision", { level: "h1" });
  const body = panelBody();
  body.append(
    el("p", "name", item.summary || item.kind),
    el("p", "meta", `Proposed by ${item.proposedBy}`),
  );
  const change = changeTable(item.changes, { before: true });
  if (change !== null) body.appendChild(change);
  if (item.evidence.length > 0) body.appendChild(evidenceList(item.evidence));
  body.appendChild(choices(item, outcomes, again));
  card.appendChild(body);
  root.appendChild(card);
}

/** The material the proposal was read out of, so the reader can check why it
 *  was staged before answering. */
function evidenceList(snippets: string[]): HTMLElement {
  const list = el("ul", "evidence-list");
  for (const snippet of snippets) list.appendChild(el("li", "meta", snippet));
  return list;
}

function choices(
  item: Item,
  outcomes: Map<string, Outcome>,
  again: () => void,
): HTMLElement {
  const outcome = outcomes.get(item.id);
  if (item.status !== "pending") {
    return settledLine(`Already ${item.status}.`);
  }
  if (outcome?.kind === "failed" && outcome.unknown === true) {
    return settledLine(outcome.reason);
  }
  if (outcome?.kind === "decided") {
    return settledLine(
      outcome.approved
        ? "Approved. The assistant still has to run it again."
        : "Rejected. Nothing was changed.",
    );
  }
  const row = el("div", "choices");
  if (item.expiresAt !== null && item.expiresAt <= Date.now()) {
    row.appendChild(badge("Expired", "warning"));
    return row;
  }
  const decide = (approve: boolean) => {
    outcomes.set(item.id, { kind: "busy" });
    again();
    void callServerTool("decide_approvals", {
      staged_action_id: item.id,
      decision: approve ? "approve" : "reject",
    }).then((result) => {
      outcomes.set(
        item.id,
        result.ok
          ? { kind: "decided", approved: approve }
          : { kind: "failed", reason: result.reason, unknown: result.unknown },
      );
      again();
    });
  };
  const busy = outcome?.kind === "busy";
  if (canCallTools()) {
    row.append(
      button("Approve", "primary", () => decide(true), busy),
      button("Reject", "ghost", () => decide(false), busy),
    );
  } else {
    row.appendChild(
      button("Ask the assistant to decide", "primary", () =>
        askAssistant(
          `Show me approval ${item.id} so I can approve or reject it.`,
        ),
      ),
    );
  }
  if (outcome?.kind === "failed") {
    row.appendChild(el("p", "refusal", outcome.reason));
  }
  return row;
}

function settledLine(text: string): HTMLElement {
  const line = el("div", "settled");
  line.appendChild(el("p", undefined, text));
  return line;
}
