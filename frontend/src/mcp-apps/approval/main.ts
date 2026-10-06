// The approval card: a change an assistant proposed and a human has to release,
// with the two answers it can be given. It draws what read_approval returns, and Approve and Reject call decide_approval through the
// host, so the same engine, authority checks and audit row apply as when the
// decision is made in the inbox.
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
import { el, onResult } from "../bridge";
import { button, panel, panelBody } from "../parts";
import { asRecord, asText, type Warning } from "../types";
import "../view.css";

declareActions(actions.approval);

type Item = {
  id: string;
  bundleID: string;
  kind: string;
  status: string;
  summary: string;
  proposedBy: string;
  expiresAt: number | null;
  rows: [string, string][];
};

type Outcome =
  | { kind: "busy" }
  | { kind: "decided"; approved: boolean }
  | { kind: "failed"; reason: string };

const decided = new WeakMap<HTMLElement, Map<string, Outcome>>();

function outcomesOf(root: HTMLElement): Map<string, Outcome> {
  let found = decided.get(root);
  if (found === undefined) {
    found = new Map();
    decided.set(root, found);
  }
  return found;
}

/** The scalar members of the staged change, which is what a reader checks
 *  before deciding. Nested values are summarised rather than dumped. */
function rowsOf(change: unknown): [string, string][] {
  return Object.entries(asRecord(change)).map(([name, value]) => {
    const text =
      typeof value === "string" || typeof value === "number"
        ? String(value)
        : typeof value === "boolean"
          ? value
            ? "yes"
            : "no"
          : "—";
    return [name.replaceAll("_", " "), text];
  });
}

function itemOf(raw: unknown): Item | null {
  const a = asRecord(raw);
  const id = asText(a.staged_action_id);
  if (id === "") return null;
  const expires = Date.parse(asText(a.expires_at));
  return {
    id,
    bundleID: asText(a.bundle_id),
    kind: asText(a.kind),
    status: asText(a.status),
    summary: asText(a.summary),
    proposedBy: asText(a.proposed_by),
    expiresAt: Number.isNaN(expires) ? null : expires,
    rows: rowsOf(a.proposed_change),
  };
}

export function render(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
): void {
  root.replaceChildren();
  const item = itemOf(data);
  if (item === null) return;
  const outcomes = outcomesOf(root);
  const again = () => render(root, data, warnings);
  const card = panel("Waiting for a decision", { level: "h1" });
  const body = panelBody();
  body.append(
    el("p", "name", item.summary || item.kind),
    el("p", "meta", `Proposed by ${item.proposedBy || "an assistant"}`),
  );
  if (item.rows.length > 0) body.appendChild(changeTable(item.rows));
  body.appendChild(choices(item, outcomes, again));
  card.appendChild(body);
  root.appendChild(card);
}

function changeTable(rows: [string, string][]): HTMLElement {
  const table = el("table", "evidence");
  const body = el("tbody");
  for (const [name, value] of rows) {
    const tr = el("tr");
    tr.append(el("th", "meta", name), el("td", "name", value));
    body.appendChild(tr);
  }
  table.appendChild(body);
  return table;
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
    void callServerTool("decide_approval", {
      staged_action_id: item.id,
      decision: approve ? "approve" : "reject",
    }).then((result) => {
      outcomes.set(
        item.id,
        result.ok
          ? { kind: "decided", approved: approve }
          : { kind: "failed", reason: result.reason },
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

onResult((data, warnings) => {
  const root = document.getElementById("root");
  if (root !== null) render(root, data, warnings);
});
