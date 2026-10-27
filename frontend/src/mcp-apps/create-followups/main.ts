// The card create_record offers after a create: what it filed for review, and a
// tag word it was asked to offer. Every create reaches this card, so a create
// with neither says what was created, and a refused one says nothing was.
//
// The duplicate panel is what create_record filed for review, with the choice the
// create itself could not offer. Merge folds the new record into the one
// already on file; "Not the same" tells the queue the pair is two contacts.
//
// Each click calls a tool the host runs under the connected assistant's own
// passport; the card shows what came back and never decides on its own what
// happened. A host that will not run a tool for a view gets one button that
// puts the same request in the chat.

import {
  askAssistant,
  callServerTool,
  canCallTools,
  declareActions,
} from "../actions";
import actions from "../actions.json";
import { el } from "../bridge";
import { fieldLabel } from "../fieldlabels";
import { button, panel, panelBody, viewRenderer } from "../parts";
import {
  changeTable,
  nameField,
  recordName,
  recordNoun,
  refusalPanel,
  summaryPanel,
  withArticle,
} from "../summary";
import {
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import { tagOfferPanel } from "./tag-offer";
import "../view.css";

declareActions(actions["create-followups"]);

/** The record types a merge folds; a lead has no merge. */
const MERGEABLE = new Set(["contact", "company"]);

type Candidate = {
  id: string;
  otherID: string;
  evidence: { field: string; mine: string; theirs: string }[];
};

type Outcome =
  | { kind: "busy" }
  | { kind: "merged" }
  | { kind: "dismissed" }
  | { kind: "failed"; reason: string; unknown?: true };

// What the reader decided, per document root and keyed by the queue's pair id,
// so a result the host redelivers into the same frame keeps it.
const decided = new WeakMap<HTMLElement, Map<string, Outcome>>();

function outcomesOf(root: HTMLElement): Map<string, Outcome> {
  let found = decided.get(root);
  if (found === undefined) {
    found = new Map();
    decided.set(root, found);
  }
  return found;
}

function candidatesOf(data: Record<string, unknown>): Candidate[] {
  return asList(data.duplicate_candidates)
    .map(asRecord)
    .map((c) => ({
      id: asText(c.candidate_id),
      otherID: asText(c.other_record_id),
      evidence: asList(c.evidence)
        .map(asRecord)
        .map((e) => ({
          field: asText(e.field),
          mine: asText(e.left_value),
          theirs: asText(e.right_value),
        })),
    }))
    .filter((c) => c.otherID !== "");
}

function evidenceTable(candidate: Candidate): HTMLElement {
  const table = el("table", "evidence");
  const head = el("tr");
  for (const label of ["", "New record", "Already on file"]) {
    head.appendChild(el("th", undefined, label));
  }
  table.appendChild(el("thead")).appendChild(head);
  const body = el("tbody");
  for (const row of candidate.evidence) {
    const tr = el("tr");
    tr.append(
      el("th", "meta", fieldLabel(row.field)),
      el("td", "name", row.mine || "—"),
      el("td", "name", row.theirs || "—"),
    );
    body.appendChild(tr);
  }
  table.appendChild(body);
  return table;
}

function settled(text: string, undo?: HTMLElement): HTMLElement {
  const line = el("div", "settled");
  line.appendChild(el("p", undefined, text));
  if (undo !== undefined) line.appendChild(undo);
  return line;
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
  if (call.refusal !== null) {
    root.appendChild(refusalPanel("Nothing was created", call.refusal));
    return;
  }
  const created = asRecord(data);
  const again = () => render(root, data, warnings, call);
  const candidates = candidatesOf(created);
  const tag = tagOfferPanel(root, created, again);
  if (candidates.length === 0) {
    root.appendChild(tag ?? createdPanel(created, call));
    return;
  }
  const recordType = asText(created.record_type);
  const createdID = asText(created.id);
  const name = recordName(asRecord(created.fields));
  const outcomes = outcomesOf(root);

  const card = panel("Next steps for this record", { level: "h1" });
  for (const candidate of candidates) {
    const body = panelBody();
    body.append(
      el(
        "p",
        "intro",
        name === ""
          ? "This record looks like one already on file."
          : `${name} looks like a record already on file.`,
      ),
      evidenceTable(candidate),
      choices(candidate, candidates, recordType, createdID, outcomes, again),
    );
    card.appendChild(body);
  }
  root.appendChild(card);
  if (tag !== null) root.appendChild(tag);
}

/** createdPanel says what a create with nothing to decide made. */
function createdPanel(
  created: Record<string, unknown>,
  call: ToolCall,
): HTMLElement {
  const fields = asRecord(created.fields);
  const noun = recordNoun(asText(created.record_type), fields);
  const skip = nameField(fields);
  const lines = [
    skip === undefined
      ? el("p", "intro", "Saved in Margince.")
      : el("p", "name", recordName(fields)),
  ];
  const rows = changeTable(asRecord(call.input.fields), fields, {
    skip,
    linked: "Set",
  });
  if (rows !== null) lines.push(rows);
  return summaryPanel(`Created ${withArticle(noun)}`, ...lines);
}

function choices(
  candidate: Candidate,
  all: Candidate[],
  recordType: string,
  createdID: string,
  outcomes: Map<string, Outcome>,
  again: () => void,
): HTMLElement {
  const outcome = outcomes.get(candidate.id);
  const settle = (next: Outcome | undefined) => {
    if (next === undefined) outcomes.delete(candidate.id);
    else if (next.kind === "merged") {
      // The created record is gone once merged, so no sibling pair can act on it.
      for (const each of all) outcomes.set(each.id, next);
    } else outcomes.set(candidate.id, next);
    again();
  };
  // A call the host never answered may have landed, so no retry is offered:
  // the reader checks first, and a second merge or dismissal would meet the
  // engine's own conflict at best.
  if (outcome?.kind === "failed" && outcome.unknown === true) {
    return settled(outcome.reason);
  }
  if (outcome?.kind === "merged") {
    return settled("Merged into the record already on file.");
  }
  if (outcome?.kind === "dismissed") {
    return settled(
      "Marked as not the same.",
      button("Undo", "ghost", () =>
        run(
          settle,
          "resolve_duplicates",
          { candidate_id: candidate.id, decision: "reopen" },
          undefined,
        ),
      ),
    );
  }
  const busy = outcome?.kind === "busy";
  const row = el("div", "choices");
  const mergeable = MERGEABLE.has(recordType);
  if (canCallTools()) {
    if (mergeable) {
      row.appendChild(
        button(
          "Merge into the existing record",
          "primary",
          () =>
            run(
              settle,
              "resolve_duplicates",
              {
                record_type: recordType,
                source_id: createdID,
                target_id: candidate.otherID,
              },
              { kind: "merged" },
            ),
          busy,
        ),
      );
    }
    row.appendChild(
      button(
        "Not the same",
        "ghost",
        () =>
          run(
            settle,
            "resolve_duplicates",
            { candidate_id: candidate.id, decision: "not_the_same" },
            { kind: "dismissed" },
          ),
        busy,
      ),
    );
  } else {
    row.appendChild(
      button("Ask the assistant to decide", "primary", () =>
        askAssistant(
          mergeable
            ? `Merge the ${recordType} ${createdID} into ${candidate.otherID}, or tell me why you would not.`
            : `Tell me whether ${createdID} and ${candidate.otherID} are the same ${recordType}.`,
        ),
      ),
    );
  }
  if (outcome?.kind === "failed") {
    row.appendChild(el("p", "refusal", outcome.reason));
  }
  return row;
}

async function run(
  settle: (next: Outcome | undefined) => void,
  tool: string,
  args: Record<string, unknown>,
  onSuccess: Outcome | undefined,
): Promise<void> {
  settle({ kind: "busy" });
  const result = await callServerTool(tool, args);
  settle(
    result.ok
      ? onSuccess
      : { kind: "failed", reason: result.reason, unknown: result.unknown },
  );
}
