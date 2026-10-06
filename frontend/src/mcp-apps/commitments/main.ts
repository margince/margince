// The commitments view: review_commitments' open promises, oldest first, with
// who owes each one and how far past its date it is — drawn as the app's own
// "Open commitments" panel, lateness in red at each row's far end.
//
// WHY THE STATE AND THE DATE TOGETHER. "Overdue" is a claim about a moment,
// and the moment is in the answer — so the foot names the instant every state
// above it was judged against. A queue of red labels with no date attached
// cannot be told from a stale panel left open since yesterday.
//
// WHAT IT REFUSES TO INVENT. A promise with no due date renders as undated,
// never as overdue and never as "due today": the answer carries no timezone
// and neither does this document, so the only honest day is the one the server
// judged in. A promise with no owner renders as unassigned, which is the state
// a reviewer is looking for rather than a blank to be filled in silently.

import { badge } from "../badge";
import { count, day, el, onResult, warned } from "../bridge";
import { panel, panelFoot, panelRow } from "../parts";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type Warning,
} from "../types";
import "../view.css";

/** The envelope's code for "this read stopped at its bound". It matters more
 *  here than on any other view: the question is whether anything is being
 *  dropped, and a silently truncated queue answers no. */
const SWEEP_TRUNCATED = "sweep_truncated";

type Commitment = {
  subject: string;
  // Which of the two places the promise was recorded. A conversation promise
  // can quote the sentence it was read from; a task carries only what somebody
  // retyped, so the two rows say different amounts about the same kind of debt.
  source: string;
  quote: string;
  state: string;
  dueAt: string;
  daysOverdue: number | null;
  owner: string;
  about: string[];
};

/**
 * known narrows the untrusted payload, filtering BEFORE anything is counted so
 * the head and the foot describe what is actually shown.
 */
function known(data: Record<string, unknown>): Commitment[] {
  return asList(data.commitments)
    .filter(
      (c): c is Record<string, unknown> => typeof c === "object" && c !== null,
    )
    .map((c) => ({
      subject: asText(c.subject) || "Untitled promise",
      source: asText(c.source),
      quote: asText(c.quote),
      state: asText(c.state),
      dueAt: asText(c.due_at),
      daysOverdue: asFiniteNumber(c.days_overdue),
      owner: asText(c.assignee_name) || asText(c.assignee_id),
      about: asList(c.about)
        .map(aboutLabel)
        .filter((label) => label !== ""),
    }));
}

/** aboutLabel names the record a promise was made about, falling back to the
 *  id where the record has no name of its own — a lead captured as an email
 *  address and nothing else. */
function aboutLabel(entry: unknown): string {
  const about = asRecord(entry);
  const name = asText(about.name) || asText(about.entity_id);
  const type = asText(about.entity_type);
  if (name === "") return "";
  return type === "" ? name : `${type}: ${name}`;
}

/** A conversation promise has no assignee to be missing — it records what was
 *  said, not who was handed it — so only a task can be unassigned. */
function unassigned(commitment: Commitment): boolean {
  return commitment.source !== "conversation" && commitment.owner === "";
}

/** lateness says how late, not merely that it is late. Zero whole days is a
 *  real answer — hours past its date — so it reads as "overdue today" rather
 *  than as "0 days". */
function lateness(days: number | null): string {
  if (days === null) return "Overdue";
  if (days === 0) return "Overdue today";
  return days === 1 ? "1 day overdue" : `${count(days)} days overdue`;
}

/** stateCell is the row's far end: lateness in the danger ink, any other state
 *  as a quiet word. A state outside the seam's vocabulary still shows, in its
 *  own word, because the vocabulary belongs to the seam. */
function stateCell(commitment: Commitment): HTMLElement {
  if (commitment.state === "overdue") {
    return el("span", "late", lateness(commitment.daysOverdue));
  }
  const word =
    commitment.state === "upcoming"
      ? "Upcoming"
      : commitment.state === "undated"
        ? "Undated"
        : commitment.state || "Unknown";
  return el("span", "meta", word);
}

function facts(commitment: Commitment): HTMLElement {
  const line = el("div", "meta");
  if (commitment.source === "conversation") {
    line.append("from a conversation");
  } else if (unassigned(commitment)) {
    line.appendChild(el("span", "unowned", "Unassigned"));
  } else {
    line.append(commitment.owner);
  }
  line.append(
    ` · ${commitment.dueAt === "" ? "no due date" : `due ${day(commitment.dueAt)}`}`,
  );
  for (const label of commitment.about) line.append(` · ${label}`);
  return line;
}

function commitmentRow(commitment: Commitment): HTMLElement {
  const row = panelRow("item item-plain");
  const main = el("div");
  main.appendChild(el("div", "name", commitment.subject));
  // The sentence the promise was made in, where there is one. It is the whole
  // reason a conversation promise is checkable: a reader can see what was
  // actually written rather than trusting a summary of it.
  if (commitment.quote !== "") {
    main.appendChild(el("div", "meta quote", `“${commitment.quote}”`));
  }
  main.appendChild(facts(commitment));
  row.append(main, stateCell(commitment));
  return row;
}

/** The foot's sentence. "Most overdue first" is only true of a COMPLETE
 *  sweep; past the bound these are the oldest promises FOUND, which is the
 *  claim the tool itself refuses to overstate. */
function footLine(asOf: string, bounded: boolean): string {
  const judged = asOf === "" ? "" : ` · judged as of ${day(asOf)}`;
  return bounded
    ? `More are outstanding than are listed here${judged}`
    : `Most overdue first${judged}`;
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
        "The host sent no structured result for this review.",
      ),
    );
    return;
  }
  const answer = asRecord(data);
  // The member has to BE an array. asList answers [] for one that is absent,
  // which would render "nothing is outstanding" — a definite, reassuring claim
  // — for a payload that is not this tool's answer at all: version skew, a
  // host dispatch error, another tool's result. An empty QUEUE and an
  // unreadable RESULT are different things and must not print the same.
  if (!Array.isArray(answer.commitments)) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no readable commitment review.",
      ),
    );
    return;
  }
  const commitments = known(answer);
  const bounded = warned(warnings, SWEEP_TRUNCATED);
  const review = panel("Open commitments", {
    level: "h1",
    action: el(
      "span",
      "meta",
      bounded ? `at least ${commitments.length}` : `${commitments.length} open`,
    ),
  });
  if (commitments.length === 0) {
    review.appendChild(
      el(
        "p",
        "empty",
        "Nothing is outstanding. That is the answer, not a gap — " +
          "though a promise made where nothing was captured is not counted.",
      ),
    );
  }
  for (const commitment of commitments) {
    review.appendChild(commitmentRow(commitment));
  }
  const foot = panelFoot();
  const overdue = commitments.filter((c) => c.state === "overdue").length;
  const nobody = commitments.filter(unassigned).length;
  if (bounded) foot.appendChild(badge("Partial", "info"));
  if (overdue > 0) foot.appendChild(badge(`${overdue} overdue`, "danger"));
  if (nobody > 0) foot.appendChild(badge(`${nobody} unassigned`, "warning"));
  foot.appendChild(
    el("p", "meta end", footLine(asText(answer.as_of), bounded)),
  );
  review.appendChild(foot);
  root.appendChild(review);
}

onResult((data, warnings) => {
  const root = document.getElementById("root");
  // Guarded rather than asserted; see the account brief for why.
  if (root !== null) render(root, data, warnings);
});
