// One staged proposal as the app's DecisionCard draws it, for the approval
// card and the approval queue. Both answer through decide_approvals, so the
// press and its outcome are here once.
//
// Approving an assistant's own proposal does not perform it: the assistant
// must run its call again with the approval id. So the card asks it to.

import {
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgencyTone,
} from "../design-system/decisiondeadline";
import { askAssistant, callServerTool, canCallTools } from "./actions";
import { badge } from "./badge";
import { el, heading } from "./bridge";
import { button, eyebrow } from "./parts";
import {
  type Change,
  changesOf,
  changeTable,
  detailsOf,
  inWords,
  type SummaryParts,
  summaryPartsOf,
} from "./summary";
import { asList, asRecord, asText } from "./types";
import { day } from "./values";

/** One staged proposal, read off a list_approvals or read_approval answer. */
export type Proposal = {
  id: string;
  bundleID: string;
  kind: string;
  status: string;
  summary: string;
  /** The summary taken apart, or null on a summary that is a sentence alone. */
  parts: SummaryParts | null;
  target: string;
  proposedBy: string;
  byAssistant: boolean;
  expiresAt: string;
  changes: Change[];
  details: Change[];
  /** The further effects approving releases, in the server's sentences. */
  effects: string[];
  /** The preview's state; "" when the answer carries none, as a listing. */
  previewState: string;
  /** The server's sentence for a preview that is not ready. */
  previewNote: string;
  evidence: string[];
};

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

/** proposalOf reads one staged action, or null for anything that is not one. */
export function proposalOf(raw: unknown): Proposal | null {
  const a = asRecord(raw);
  const id = asText(a.staged_action_id);
  if (id === "") return null;
  const proposedBy = asText(a.proposed_by);
  return {
    id,
    bundleID: asText(a.bundle_id),
    kind: asText(a.kind),
    status: asText(a.status),
    summary: asText(a.summary),
    parts: summaryPartsOf(a.summary_parts),
    target: asText(a.target_label),
    proposedBy: proposerOf(proposedBy, a.on_behalf_of),
    byAssistant: proposedBy.startsWith("agent:"),
    expiresAt: asText(a.expires_at),
    changes: changesOf(a.changes),
    details: detailsOf(a.details),
    effects: asList(a.effects)
      .map((e) => asText(asRecord(e).label))
      .filter((label) => label !== ""),
    previewState: asText(a.preview_state),
    previewNote: asText(a.preview_note),
    evidence: asList(a.evidence)
      .map((e) => asText(asRecord(e).evidence_snippet))
      .filter((snippet) => snippet !== ""),
  };
}

/** previewBlocks reports a preview the server will not let anyone approve
 *  from, such as one touching records the reader cannot open. */
function previewBlocks(p: Proposal): boolean {
  return p.previewState !== "" && p.previewState !== "ready";
}

/** isOpen reports whether a proposal can still be answered at `now`. */
export function isOpen(p: Proposal, now: number): boolean {
  return (
    p.status === "pending" &&
    !previewBlocks(p) &&
    !decisionLapsed({ status: p.status, expires_at: p.expiresAt }, now)
  );
}

/** What a press on one proposal came to. */
export type Outcome =
  | { kind: "busy" }
  | { kind: "decided"; approved: boolean }
  | { kind: "failed"; reason: string; unknown?: true };

let nextHeadlineID = 1;

const decided = new WeakMap<HTMLElement, Map<string, Outcome>>();

/** outcomesOf keeps what the reader decided per document root, so a result
 *  the host redelivers into the same frame keeps it. */
export function outcomesOf(root: HTMLElement): Map<string, Outcome> {
  let found = decided.get(root);
  if (found === undefined) {
    found = new Map();
    decided.set(root, found);
  }
  return found;
}

/**
 * decide answers proposals through decide_approvals: one by its id, or a
 * bundle's waiting members together. Each proposal's outcome is set before the
 * call and after it, and `again` redraws the card each time.
 */
export function decide(
  answer: Readonly<{
    approve: boolean;
    proposals: Proposal[];
    bundle?: string;
  }>,
  outcomes: Map<string, Outcome>,
  again: () => void,
): void {
  for (const p of answer.proposals) outcomes.set(p.id, { kind: "busy" });
  again();
  const target =
    answer.bundle === undefined
      ? { staged_action_id: answer.proposals[0]?.id }
      : { bundle_id: answer.bundle };
  void callServerTool("decide_approvals", {
    ...target,
    decision: answer.approve ? "approve" : "reject",
  }).then((result) => {
    for (const p of answer.proposals) {
      outcomes.set(
        p.id,
        result.ok
          ? { kind: "decided", approved: answer.approve }
          : { kind: "failed", reason: result.reason, unknown: result.unknown },
      );
    }
    again();
    if (result.ok && answer.approve) handBack(answer.proposals);
  });
}

/** handBack asks the assistant to run the calls it was waiting to have
 *  approved. A colleague's or an automation's proposal runs on approval. The
 *  message names only the kind and id: a summary can carry outside text. */
function handBack(proposals: Proposal[]): void {
  const own = proposals.filter((p) => p.byAssistant);
  if (own.length === 0) return;
  const named = own
    .map((p) => `${inWords(p.kind)} (approval_id ${p.id})`)
    .join(", ");
  askAssistant(
    `I approved ${named}. Run the approved call again with its approval_id now.`,
  );
}

/** countdown says how long a proposal has left, in the urgency's tone. */
function countdown(p: Proposal, now: number): HTMLElement | null {
  const at = decisionExpiryMs({ status: p.status, expires_at: p.expiresAt });
  if (at === null || Number.isNaN(at) || p.status !== "pending") return null;
  const left = at - now;
  if (left <= 0) return badge("Expired", "warning");
  const hours = Math.floor(left / 3_600_000);
  const words =
    hours >= 48
      ? `Expires ${day(p.expiresAt)}`
      : hours >= 1
        ? `Expires in ${hours} h`
        : `Expires in ${Math.max(1, Math.floor(left / 60_000))} min`;
  return badge(words, decisionUrgencyTone(left) ?? "default");
}

/** The verdict words for a proposal already answered elsewhere. */
type VerdictTone = "success" | "danger" | "warning";

const VERDICT: Readonly<Record<string, [string, VerdictTone]>> = {
  approved: ["Approved", "success"],
  rejected: ["Rejected", "danger"],
};

function statusBadge(p: Proposal): HTMLElement | null {
  if (p.status === "pending") return null;
  const [word, tone]: [string, VerdictTone] = VERDICT[p.status] ?? [
    inWords(p.status),
    "warning",
  ];
  return badge(word, tone);
}

/** proposalBody is what approving does: the changes, the operands and the
 *  further effects. It answers null when the answer names none of them. */
function proposalBody(p: Proposal): HTMLElement | null {
  const body = el("div", "dcard-body");
  for (const table of [
    changeTable(p.changes, { before: true }),
    changeTable(p.details, { before: false }),
  ]) {
    if (table !== null) body.appendChild(table);
  }
  // The summary's own values, where the preview shows none, so nothing is
  // listed twice.
  if (body.childElementCount === 0 && p.parts !== null) {
    const table = changeTable(p.parts.fields, { before: false });
    if (table !== null) body.appendChild(table);
    if (p.parts.more > 0)
      body.appendChild(el("p", "meta", `${p.parts.more} more not shown`));
  }
  if (p.effects.length > 0) {
    const list = el("ul", "evidence-list");
    for (const effect of p.effects) list.appendChild(el("li", "meta", effect));
    body.append(eyebrow("Also changes"), list);
  }
  return body.childElementCount > 0 ? body : null;
}

/**
 * decisionCard mirrors `DecisionCard`: the staged ground and the deadline band.
 * Over the headline sit the kind and target; under it, the change, its
 * evidence and the two answers.
 */
export function decisionCard(
  p: Proposal,
  opts: Readonly<{
    layout: "deck" | "row";
    now: number;
    outcome: Outcome | undefined;
    onDecide: (approve: boolean) => void;
  }>,
): HTMLElement {
  const card = el("article", "staging-card dcard");
  card.dataset.layout = opts.layout;
  card.dataset.approval = p.id;
  const open = isOpen(p, opts.now);
  const meta = el("div", "dcard-meta");
  meta.appendChild(eyebrow(inWords(p.kind)));
  for (const chip of [
    p.target === "" ? null : badge(p.target),
    statusBadge(p),
    countdown(p, opts.now),
  ]) {
    if (chip !== null) meta.appendChild(chip);
  }
  // The one card of a deck is the document's heading; a row is one of many.
  const words = p.parts?.sentence || p.summary || inWords(p.kind);
  const headline =
    opts.layout === "deck"
      ? heading("large", words, { as: "h1", className: "approval-headline" })
      : el("p", "t-body approval-headline", words);
  headline.id = `decision-headline-${nextHeadlineID++}`;
  card.setAttribute("aria-labelledby", headline.id);
  card.append(meta, headline, el("p", "meta", `Proposed by ${p.proposedBy}`));
  const body = proposalBody(p);
  if (body !== null) card.appendChild(body);
  if (p.evidence.length > 0) {
    const evidence = el("div", "dcard-evidence");
    const list = el("ul", "evidence-list");
    for (const snippet of p.evidence)
      list.appendChild(el("li", "meta", snippet));
    evidence.appendChild(list);
    card.appendChild(evidence);
  }
  if (p.status === "pending") card.appendChild(verbs(p, open, opts));
  return card;
}

function verbs(
  p: Proposal,
  open: boolean,
  opts: Readonly<{
    outcome: Outcome | undefined;
    onDecide: (a: boolean) => void;
  }>,
): HTMLElement {
  const outcome = opts.outcome;
  if (outcome?.kind === "decided") {
    return settledLine(
      !outcome.approved
        ? "Rejected. Nothing was changed."
        : p.byAssistant
          ? "Approved. The assistant is running it now."
          : "Approved.",
    );
  }
  if (outcome?.kind === "failed" && outcome.unknown === true) {
    return settledLine(outcome.reason);
  }
  if (previewBlocks(p)) {
    return settledLine(
      p.previewNote || "This can no longer be approved as it stands.",
    );
  }
  if (!open)
    return settledLine("This ran out of time and can no longer be answered.");
  const row = el("div", "dcard-verbs choices");
  const busy = outcome?.kind === "busy";
  if (canCallTools()) {
    row.append(
      button("Reject", "ghost", () => opts.onDecide(false), busy),
      button("Approve", "primary", () => opts.onDecide(true), busy),
    );
  } else {
    row.appendChild(
      button("Ask the assistant to decide", "primary", () =>
        askAssistant(`Show me approval ${p.id} so I can approve or reject it.`),
      ),
    );
  }
  if (outcome?.kind === "failed") {
    row.appendChild(el("p", "refusal", outcome.reason));
  }
  return row;
}

/** settledLine is where the answers were, once there is nothing to answer. */
export function settledLine(text: string): HTMLElement {
  const line = el("div", "dcard-verbs settled");
  line.appendChild(el("p", undefined, text));
  return line;
}
