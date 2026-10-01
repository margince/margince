// The handoff view: prepare_handoff's briefing for one project, drawn as the
// app's project page draws one — the record's head, then its Deals,
// Stakeholders and Open commitments — with the verdict and its gaps between.
//
// WHY THE GAPS COME FIRST. This panel is read to answer one question — is this
// work ready to hand over — and the answer is the list of what is missing. A
// brief that led with what it HAS reads as complete, which is exactly the
// failure the tool's gaps exist to prevent: an absent owner looks identical to
// a present one when nothing points at the absence.
//
// EVERY GAP SHOWS THE FIELD IT WAS READ OFF. The tool refuses to raise a gap
// it cannot point at a field for, and this document keeps that promise
// visible: a warning with no source beside it is advice, and neither the tool
// nor the view gives advice.

import { badge } from "../badge";
import { ABSENT, day, el, heading, money, onResult, warned } from "../bridge";
import {
  avatar,
  callout,
  panel,
  panelBody,
  panelFoot,
  panelRow,
} from "../parts";
import { asList, asRecord, asText, type Warning } from "../types";
import "../view.css";

/** The envelope's code for "a list in this answer stopped at its bound". The
 *  tool withholds the gaps a bounded read cannot support and says so with
 *  this, so a view that dropped it would present a briefing with checks
 *  MISSING as a briefing with nothing missing. */
const SWEEP_TRUNCATED = "sweep_truncated";

/** The phases a project moves through, as the project page words them. */
const PHASES: ReadonlyMap<string, string> = new Map([
  ["initiative", "Initiative"],
  ["pursuing", "Pursuing"],
  ["delivering", "Delivering"],
  ["closed", "Closed"],
]);

type Gap = { message: string; source: string };
type Deal = { name: string; status: string; amount: string };
type Seat = { contact: string; contactID: string; role: string };
type Promise_ = { subject: string; state: string; dueAt: string };

/** The gaps, and how many were unreadable.
 *
 *  THE DROP COUNT IS RETURNED, not swallowed. A gap with no message cannot be
 *  rendered, and if EVERY gap is unreadable the list goes empty — which on this
 *  panel is not "nothing to report" but the verdict "ready to hand over". A
 *  drop has to be able to withhold that verdict, so the caller is told there
 *  was one. */
function gapsOf(data: Record<string, unknown>): {
  gaps: Gap[];
  dropped: number;
} {
  const read = asList(data.gaps)
    .map((entry) => asRecord(entry))
    .map((gap) => ({
      message: asText(gap.message),
      source: asText(gap.source),
    }));
  const gaps = read.filter((gap) => gap.message !== "");
  return { gaps, dropped: read.length - gaps.length };
}

function dealsOf(data: Record<string, unknown>): Deal[] {
  return asList(data.deals)
    .map((entry) => asRecord(entry))
    .map((deal) => ({
      name: asText(deal.name) || asText(deal.deal_id),
      status: asText(deal.status) || "unknown status",
      // Absent, not zero: a deal can be won before it is priced, which is one
      // of the gaps the verdict above may already be reporting.
      amount: money(deal.amount_minor, deal.currency),
    }));
}

function seatsOf(data: Record<string, unknown>): Seat[] {
  return asList(data.stakeholders)
    .map((entry) => asRecord(entry))
    .map((seat) => ({
      // The name where the answer has one, the id where it does not — a seat
      // whose contact the caller may not read comes back unnamed, and an id is
      // a worse answer than a name but a much better one than a blank.
      contact: asText(seat.name) || asText(seat.contact_id),
      contactID: asText(seat.contact_id),
      role: asText(seat.role),
    }))
    .filter((seat) => seat.contact !== "");
}

function promisesOf(data: Record<string, unknown>): Promise_[] {
  return asList(data.open_commitments)
    .map((entry) => asRecord(entry))
    .map((promise) => ({
      subject: asText(promise.subject) || "Untitled task",
      state: asText(promise.state),
      dueAt: asText(promise.due_at),
    }));
}

/** phaseBadge reads as the project page's: a live phase in the success tone,
 *  a closed one neutral. A phase outside the set shows in its own word and
 *  the neutral tone, because a tone is a claim nobody made about it. */
function phaseBadge(phase: string): HTMLElement {
  const word = PHASES.get(phase);
  if (word === undefined) return badge(phase);
  return badge(word, phase === "closed" ? "default" : "success");
}

/** fact is one labelled value in the record's head; an absent one is said in
 *  the warning ink, because it is a gap the verdict below names too. */
function fact(label: string, value: string, missing: string): HTMLElement {
  const cell = el("div");
  cell.append(
    el("div", "meta", label),
    value === "" ? el("div", "unowned", missing) : el("div", undefined, value),
  );
  return cell;
}

/** head is the project's own panel: its mark, name, phase and key, then who
 *  receives the work and when it is meant to end. */
function head(answer: Record<string, unknown>): HTMLElement {
  const block = el("section", "panel");
  const body = panelBody();
  const line = el("div", "record-line");
  const name = asText(answer.name) || "Delivery handoff";
  line.append(
    avatar(name, asText(answer.project_id), "md"),
    heading("large", name, { as: "h1", className: "name" }),
  );
  const phase = asText(answer.phase);
  line.appendChild(
    phase === "" ? el("span", "meta", "no phase") : phaseBadge(phase),
  );
  const key = asText(answer.key);
  if (key !== "") line.appendChild(el("span", "meta", `# ${key}`));
  body.appendChild(line);
  const facts = el("div", "record-facts");
  const target = asText(answer.target_end_date);
  // The name where the answer has one, the id where it does not — the same
  // fallback the seats take, for the same reason.
  const owner = asText(answer.owner_name) || asText(answer.owner_id);
  const started = asText(answer.started_at);
  facts.append(
    fact("Target end date", target === "" ? "" : day(target), "Not set"),
    fact("Owner", owner, "Unassigned"),
    fact("Started", started === "" ? "" : day(started), "Not recorded"),
  );
  body.appendChild(facts);
  block.appendChild(body);
  return block;
}

/** What a bounded read costs this briefing, in the tool's own terms. */
const boundedNote =
  "The lists below stopped at their bound, so they are partial — and the " +
  "checks for an absent won deal or an absent contact were withheld rather " +
  "than guessed.";

/**
 * verdict is the panel's answer to the one question it is opened for: is this
 * work ready to hand over.
 *
 * "Ready" is the strongest claim here, and it is only true when every check
 * RAN and every result was readable. A bounded read had checks withheld by the
 * tool; an unreadable gap had one lost in transit. Either way the honest
 * answer is that the question was not fully answered — which is a different
 * thing from the answer being "yes".
 */
function verdict(
  answer: Record<string, unknown>,
  bounded: boolean,
): HTMLElement {
  const { gaps, dropped } = gapsOf(answer);
  if (gaps.length > 0) {
    const missing = panel("Not ready to hand over", {
      tone: "warning",
      action: badge(`${gaps.length} missing`, "warning"),
    });
    for (const gap of gaps) {
      const row = panelRow("gap");
      row.appendChild(el("div", "name", gap.message));
      if (gap.source !== "") {
        row.appendChild(el("div", "meta source", `read from ${gap.source}`));
      }
      missing.appendChild(row);
    }
    if (bounded) {
      const foot = panelFoot();
      foot.appendChild(el("p", "meta", boundedNote));
      missing.appendChild(foot);
    }
    return missing;
  }
  if (bounded || dropped > 0) {
    return callout(
      "info",
      "Can't confirm it's ready",
      "Not every check could be made, so this briefing cannot say the work " +
        "is ready to hand over. " +
        (bounded ? boundedNote : "A reported gap arrived unreadable."),
    );
  }
  return callout(
    "success",
    "Ready to hand over",
    "Nothing the records were checked for is missing.",
  );
}

/** section is one of the project page's panels, or nothing when it would be
 *  empty — a title over a void, where the verdict has already said which
 *  absences matter. */
function section(title: string, rows: HTMLElement[]): HTMLElement | null {
  if (rows.length === 0) return null;
  const block = panel(title);
  for (const row of rows) block.appendChild(row);
  return block;
}

function dealRow(deal: Deal): HTMLElement {
  const row = panelRow("item item-plain");
  row.appendChild(el("div", "name", deal.name));
  const end = el("div", "item-title");
  const tone =
    deal.status === "won"
      ? "success"
      : deal.status === "lost"
        ? "danger"
        : "default";
  end.append(
    badge(deal.status, tone),
    el(
      "span",
      deal.amount === ABSENT ? "figure figure-absent" : "figure",
      deal.amount,
    ),
  );
  row.appendChild(end);
  return row;
}

function seatRow(seat: Seat): HTMLElement {
  const row = panelRow("item");
  row.append(
    avatar(seat.contact, seat.contactID),
    el("div", "name", seat.contact),
    // An untitled seat is a gap the verdict names, so the row agrees with it
    // rather than leaving an empty cell.
    seat.role === "" ? badge("No role recorded", "warning") : badge(seat.role),
  );
  return row;
}

function promiseRow(promise: Promise_): HTMLElement {
  const row = panelRow("item item-plain");
  const main = el("div");
  main.appendChild(el("div", "name", promise.subject));
  const facts = el("div", "item-title");
  facts.appendChild(
    el(
      "span",
      "meta",
      promise.dueAt === "" ? "no due date" : `due ${day(promise.dueAt)}`,
    ),
  );
  // An overdue promise at handover is the one that follows the work across.
  if (promise.state === "overdue")
    facts.appendChild(badge("Overdue", "danger"));
  main.appendChild(facts);
  row.appendChild(main);
  return row;
}

export function render(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
): void {
  root.replaceChildren();
  if (data === null || data === undefined) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no structured result for this project.",
      ),
    );
    return;
  }
  const answer = asRecord(data);
  // A payload that is not a handoff is refused rather than narrowed into one.
  // Every other member degrades quietly to an empty list, and an answer with
  // no gaps renders as "ready to hand over" — so a number, a string or another
  // tool's result would be shown to a human as a project cleared for handover.
  //
  // Both members are checked. `project_id` is the one the tool always answers,
  // and `gaps` is the one whose emptiness this panel reads as its verdict; the
  // tool always serializes it, an empty array at worst, so an absent member is
  // proof of skew rather than of a clean project.
  if (asText(answer.project_id) === "" || !Array.isArray(answer.gaps)) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no readable handoff for this project.",
      ),
    );
    return;
  }
  const page = el("div", "stack");
  page.append(head(answer), verdict(answer, warned(warnings, SWEEP_TRUNCATED)));
  for (const block of [
    section("Deals", dealsOf(answer).map(dealRow)),
    section("Stakeholders", seatsOf(answer).map(seatRow)),
    section("Open commitments", promisesOf(answer).map(promiseRow)),
  ]) {
    if (block !== null) page.appendChild(block);
  }
  root.appendChild(page);
}

onResult((data, warnings) => {
  const root = document.getElementById("root");
  // Guarded rather than asserted; see the account brief for why.
  if (root !== null) render(root, data, warnings);
});
