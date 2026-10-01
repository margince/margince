// The morning brief view: read_brief's queue, drawn as the app draws its own
// ranked work — the AI-assisted panel, a rank chip per row — with the factor
// decomposition each item ranked on.
//
// WHY THE FACTORS ARE THE POINT. The brief's own contract forbids the mystery
// number — an item that says only "this ranked first" restates the queue, while
// one that says "first on momentum and warmth" has told the contact something.
// The score alone would fit in the chat text this view renders instead of; the
// five meters beside it are what a panel buys.
//
// It renders and nothing else: no control, no action, no call back into the
// surface. Acting on a brief item is a human-only route by contract, so a button
// here would be a door the contract does not have.

import { badge } from "../badge";
import { count, day, el, onResult, percent } from "../bridge";
import { meter, panel, panelFoot, panelRow } from "../parts";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type Warning,
} from "../types";
import "../view.css";

/** The factor keys the seam publishes, with the word each is shown under. */
const FACTORS: ReadonlyArray<readonly [string, string]> = [
  ["winnability", "Winnability"],
  ["revenue", "Revenue"],
  ["timing", "Timing"],
  ["momentum", "Momentum"],
  ["warmth", "Warmth"],
];

/** The queue states a reader has left an item in, as the Worklist words them.
 *  `new` is absent on purpose: an item nobody has touched carries no badge. */
const STATES: ReadonlyMap<string, readonly [string, "default" | "success"]> =
  new Map([
    ["snoozed", ["Snoozed", "default"]],
    ["dismissed", ["Dismissed", "default"]],
    ["acted", ["Done", "success"]],
  ]);

/** One queue entry, after narrowing. Every member is what the view will draw,
 *  never what the host happened to send. */
type Item = {
  dealID: string;
  rank: unknown;
  composite: number | null;
  factors: Record<string, unknown>;
  state: string;
};

/**
 * queued narrows the untrusted payload to the rows this view will actually
 * draw, and it filters BEFORE anything is counted — a foot describing rows
 * that were then skipped is a view claiming an answer it did not show.
 */
function queued(data: Record<string, unknown>): Item[] {
  return asList(data.items)
    .filter(
      (i): i is Record<string, unknown> => typeof i === "object" && i !== null,
    )
    .map((i) => ({
      dealID: asText(i.deal_id),
      rank: i.rank,
      composite: asFiniteNumber(i.composite),
      factors: asRecord(i.factors),
      state: asText(i.state),
    }));
}

function factorCell(label: string, value: unknown): HTMLElement {
  const cell = el("div", "factor");
  const line = el("div", "factor-line");
  line.append(
    meter(asFiniteNumber(value), label),
    el("span", "meta figure", percent(value)),
  );
  cell.append(el("div", "meta factor-name", label), line);
  return cell;
}

/** stateBadge names a queue state the reader set, or nothing for a new item.
 *  A state outside the set still shows, in its own word, because the
 *  vocabulary belongs to the seam. */
function stateBadge(state: string): HTMLElement | null {
  if (state === "" || state === "new") return null;
  const known = STATES.get(state);
  return known === undefined ? badge(state) : badge(known[0], known[1]);
}

// The deal is titled the way the Worklist titles a brief item it cannot name,
// and identified by its id beneath: a brief item carries no deal name, and
// inventing a lookup for one would be this view introducing a data path —
// which is exactly what an App must not do.
function itemRow(item: Item): HTMLElement {
  const row = panelRow("item item-ranked");
  row.appendChild(el("span", "rank", count(item.rank)));
  const main = el("div");
  const title = el("div", "item-title");
  title.appendChild(el("span", "name", "Deal to review"));
  const state = stateBadge(item.state);
  if (state !== null) title.appendChild(state);
  title.appendChild(
    el(
      "span",
      "meta score end",
      item.composite === null
        ? "No score"
        : `Score ${Math.round(item.composite * 100)} of 100`,
    ),
  );
  main.appendChild(title);
  if (item.dealID !== "") {
    main.appendChild(el("div", "meta ref", `deal ${item.dealID}`));
  }
  const factors = el("div", "factors");
  for (const [key, label] of FACTORS) {
    factors.appendChild(factorCell(label, item.factors[key]));
  }
  main.appendChild(factors);
  row.appendChild(main);
  return row;
}

export function render(
  root: HTMLElement,
  data: unknown,
  _warnings: Warning[],
): void {
  // Replacing children rather than clearing markup: a view may be sent a second
  // result, and the first one's nodes have to go without any string ever being
  // parsed as markup.
  root.replaceChildren();
  if (data === null || data === undefined) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no structured result for this brief.",
      ),
    );
    return;
  }
  const answer = asRecord(data);
  const items = queued(answer);
  const brief = panel("Morning brief", {
    tone: "ai",
    level: "h1",
    action: badge("AI-assisted", "ai"),
  });
  if (items.length === 0) {
    brief.appendChild(
      el(
        "p",
        "empty",
        "Nothing is queued. An empty brief is an answer, not a failure.",
      ),
    );
  }
  for (const item of items) brief.appendChild(itemRow(item));
  // candidate_count already reports what the ranking left out, which is this
  // view's whole completeness story — read_brief raises no truncation warning,
  // so there is no second condition to surface and no branch here for one.
  const foot = panelFoot();
  const asOf = asText(answer.as_of);
  foot.appendChild(
    el(
      "p",
      "meta",
      `${items.length} of ${count(answer.candidate_count)} candidates shown` +
        (asOf === "" ? "" : ` · as of ${day(asOf)}`),
    ),
  );
  brief.appendChild(foot);
  root.appendChild(brief);
}

onResult((data, warnings) => {
  const root = document.getElementById("root");
  // Guarded rather than asserted: the root is this document's own element, but
  // an assertion here would be the view promising something about a page it may
  // one day not be the only script on.
  if (root !== null) render(root, data, warnings);
});
