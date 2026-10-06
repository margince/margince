// The field-conflict card: an update that touched fields last edited by hand.
// The rest of the patch applied; these fields wait, because an assistant does
// not silently overwrite what someone typed. The card shows the value on the
// record beside the value proposed, and the two answers.
//
// "Keep" rejects the staged change. "Use the new values" approves it and then
// sends update_record with exactly the replay the result carried and the
// approval's id, which is the redemption the engine documents. The two steps
// can fail apart, so an approval that landed without its update leaves one
// button that finishes it.

import {
  askAssistant,
  callServerTool,
  canCallTools,
  declareActions,
} from "../actions";
import actions from "../actions.json";
import { el, onResult } from "../bridge";
import { button, panel, panelBody } from "../parts";
import { asList, asRecord, asText, type Warning } from "../types";
import "../view.css";

declareActions(actions["field-conflict"]);

type Conflict = {
  approvalID: string;
  fields: { name: string; current: string; proposed: string }[];
  replay: Record<string, unknown>;
};

type Outcome =
  | { kind: "busy" }
  | { kind: "kept" }
  | { kind: "updated" }
  | { kind: "approved"; reason?: string }
  | { kind: "failed"; reason: string };

// What the reader decided, per document root and keyed by the approval, so a
// later conflict drawn into the same frame starts undecided.
const decided = new WeakMap<HTMLElement, Map<string, Outcome>>();

function outcomesOf(root: HTMLElement): Map<string, Outcome> {
  let found = decided.get(root);
  if (found === undefined) {
    found = new Map();
    decided.set(root, found);
  }
  return found;
}

/** shown renders a stored value as the text a reader sees. */
function shown(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "string" || typeof value === "number")
    return String(value);
  if (typeof value === "boolean") return value ? "yes" : "no";
  return JSON.stringify(value);
}

function conflictOf(data: unknown): Conflict | null {
  const record = asRecord(data);
  const staged = asRecord(record.staged_approval);
  const approvalID = asText(staged.approval_id);
  const names = asList(staged.fields)
    .map(asText)
    .filter((n) => n !== "");
  if (approvalID === "" || names.length === 0) return null;
  const replay = asRecord(staged.replay);
  const current = asRecord(record.fields);
  const proposed = asRecord(replay.fields);
  return {
    approvalID,
    replay,
    fields: names.map((name) => ({
      name,
      current: shown(current[name]),
      proposed: shown(proposed[name]),
    })),
  };
}

function table(conflict: Conflict): HTMLElement {
  const grid = el("table", "evidence");
  const head = el("tr");
  for (const label of ["", "On the record", "Proposed"]) {
    head.appendChild(el("th", undefined, label));
  }
  grid.appendChild(el("thead")).appendChild(head);
  const body = el("tbody");
  for (const field of conflict.fields) {
    const tr = el("tr");
    tr.append(
      el("th", "meta", field.name.replaceAll("_", " ")),
      el("td", "name", field.current),
      el("td", "name", field.proposed),
    );
    body.appendChild(tr);
  }
  grid.appendChild(body);
  return grid;
}

export function render(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
): void {
  root.replaceChildren();
  const conflict = conflictOf(data);
  if (conflict === null) return;
  const again = () => render(root, data, warnings);
  const outcomes = outcomesOf(root);
  const set = (next: Outcome) => {
    outcomes.set(conflict.approvalID, next);
    again();
  };
  const card = panel("Edited by hand", { level: "h1" });
  const body = panelBody();
  body.append(
    el(
      "p",
      "intro",
      "The rest of the change was applied. These fields were last edited by hand, so the change is waiting for your answer.",
    ),
    table(conflict),
    choices(conflict, outcomes.get(conflict.approvalID), set),
  );
  card.appendChild(body);
  root.appendChild(card);
}

function choices(
  conflict: Conflict,
  outcome: Outcome | undefined,
  set: (next: Outcome) => void,
): HTMLElement {
  if (outcome?.kind === "kept")
    return settledLine("Kept the values on the record.");
  if (outcome?.kind === "updated")
    return settledLine("Updated with the new values.");
  const row = el("div", "choices");
  const busy = outcome?.kind === "busy";
  if (!canCallTools()) {
    row.appendChild(
      button("Ask the assistant to decide", "primary", () =>
        askAssistant(
          `Show me approval ${conflict.approvalID} so I can keep the current values or use the new ones.`,
        ),
      ),
    );
    return row;
  }
  const apply = () => finish(conflict, set);
  if (outcome?.kind === "approved") {
    row.appendChild(button("Apply the new values", "primary", apply, busy));
  } else {
    row.append(
      button(
        "Keep the current values",
        "ghost",
        () => keep(conflict, set),
        busy,
      ),
      button("Use the new values", "primary", () => use(conflict, set), busy),
    );
  }
  if (
    outcome?.kind === "failed" ||
    (outcome?.kind === "approved" && outcome.reason)
  ) {
    const reason =
      outcome.kind === "failed" ? outcome.reason : (outcome.reason ?? "");
    row.appendChild(el("p", "refusal", reason));
  }
  return row;
}

async function keep(
  conflict: Conflict,
  set: (next: Outcome) => void,
): Promise<void> {
  set({ kind: "busy" });
  const result = await callServerTool("decide_approval", {
    staged_action_id: conflict.approvalID,
    decision: "reject",
  });
  set(result.ok ? { kind: "kept" } : { kind: "failed", reason: result.reason });
}

async function use(
  conflict: Conflict,
  set: (next: Outcome) => void,
): Promise<void> {
  set({ kind: "busy" });
  const result = await callServerTool("decide_approval", {
    staged_action_id: conflict.approvalID,
    decision: "approve",
  });
  if (!result.ok) {
    // A call the host never answered may have approved: offer the finishing
    // step instead of a retry that would meet "already answered".
    set(
      result.unknown === true
        ? { kind: "approved", reason: result.reason }
        : { kind: "failed", reason: result.reason },
    );
    return;
  }
  await finish(conflict, set);
}

async function finish(
  conflict: Conflict,
  set: (next: Outcome) => void,
): Promise<void> {
  set({ kind: "busy" });
  const result = await callServerTool("update_record", {
    ...conflict.replay,
    approval_id: conflict.approvalID,
  });
  set(
    result.ok
      ? { kind: "updated" }
      : { kind: "approved", reason: result.reason },
  );
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
