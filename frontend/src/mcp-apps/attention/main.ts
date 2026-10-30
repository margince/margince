// The attention card: what read_attention answered, as a ranked list a reader
// scans. One tool answers three questions, and the answer's shape says which:
// deals slipping this week, relationships going cold, or promises still open.
//
// Each row's "Catch me up" puts the follow-up question in the chat. The card
// calls no tool itself, so it declares no actions.

import { askAssistant } from "../actions";
import { badge } from "../badge";
import { el } from "../bridge";
import {
  avatar,
  button,
  panel,
  panelBody,
  type Stat,
  statStrip,
  viewRenderer,
} from "../parts";
import { refusalPanel } from "../summary";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import { money } from "../values";
import "../view.css";

/** render draws the card, keeping the reader's place across a redraw. */
export const render = viewRenderer(draw);

function draw(
  root: HTMLElement,
  data: unknown,
  _warnings: Warning[],
  call: ToolCall,
): void {
  root.replaceChildren();
  if (call.refusal !== null) {
    root.appendChild(refusalPanel("Nothing to show", call.refusal));
    return;
  }
  const answer = asRecord(data);
  if (Array.isArray(answer.commitments)) {
    root.appendChild(commitmentsCard(answer));
  } else if ("deals_scanned" in answer) {
    root.appendChild(atRiskCard(answer));
  } else if (Array.isArray(answer.deals)) {
    root.appendChild(slippingCard(answer));
  } else {
    root.appendChild(refusalPanel("Nothing to show", null));
  }
}

/** The card's frame: the title, a count beside it, and the figures under it. */
function frame(
  title: string,
  count: number,
  stats: Stat[],
  floor?: string,
): { card: HTMLElement; body: HTMLElement } {
  const card = panel(title, {
    level: "h1",
    action: badge(String(count), count > 0 ? "warning" : "success"),
  });
  const body = panelBody();
  if (count > 0 || floor !== undefined)
    body.appendChild(statStrip(stats, floor));
  card.appendChild(body);
  return { card, body };
}

/** A ranked row: the marker, what it is and why, its figure, and the door. */
function row(
  marker: HTMLElement,
  name: string,
  reasons: HTMLElement[],
  figure: string,
  question: string,
): HTMLElement {
  const item = el("li", "panel-row item attention-row");
  const text = el("div", "attention-text");
  text.appendChild(el("p", "name", name));
  const why = el("div", "attention-reasons");
  why.append(...reasons);
  text.appendChild(why);
  const end = el("div", "attention-end");
  if (figure !== "") end.appendChild(el("span", "figure", figure));
  end.appendChild(button("Catch me up", "ghost", () => askAssistant(question)));
  item.append(marker, text, end);
  return item;
}

function rankMarker(rank: number): HTMLElement {
  const node = el("span", "attention-rank figure", rank);
  node.setAttribute("aria-label", `Rank ${rank}`);
  return node;
}

function slippingCard(answer: Record<string, unknown>): HTMLElement {
  const deals = asList(answer.deals).map(asRecord);
  const { card, body } = frame(
    "Slipping this week",
    deals.length,
    slippingFigures(deals),
  );
  if (deals.length === 0) {
    body.appendChild(el("p", "intro", "No deal is slipping this week."));
    return card;
  }
  const list = el("ol");
  deals.forEach((deal, index) => {
    const name = asText(deal.name) || "Unnamed deal";
    const reasons = asList(deal.evidence)
      .map(asRecord)
      .map((e) => reasonLine(asText(e.snippet)));
    list.appendChild(
      row(
        rankMarker(asFiniteNumber(deal.rank) ?? index + 1),
        name,
        reasons,
        deal.amount_minor === undefined
          ? ""
          : money(deal.amount_minor, deal.currency),
        `Catch me up on the deal "${name}" (${asText(deal.deal_id)}): what happened and what should I do next?`,
      ),
    );
  });
  card.appendChild(list);
  return card;
}

/** What is at stake: one total when every amount is in one currency. */
function slippingFigures(deals: Record<string, unknown>[]): Stat[] {
  const priced = deals.filter(
    (d) => asFiniteNumber(d.amount_minor) !== null && asText(d.currency) !== "",
  );
  const currencies = new Set(priced.map((d) => asText(d.currency)));
  const stats: Stat[] = [
    { label: "Deals slipping", value: String(deals.length), tone: "warning" },
  ];
  const [only] = [...currencies];
  if (currencies.size === 1 && only !== undefined) {
    const total = priced.reduce(
      (sum, d) => sum + (asFiniteNumber(d.amount_minor) ?? 0),
      0,
    );
    stats.push({ label: "Value at stake", value: money(total, only) });
  } else if (currencies.size > 1) {
    stats.push({
      label: "Value at stake",
      value: "Mixed",
      detail: `In ${currencies.size} currencies`,
    });
  }
  const quiet = deals.filter((d) =>
    asList(d.evidence).some((e) =>
      ["deal.last_activity_at", "deal.created_at"].includes(
        asText(asRecord(e).source),
      ),
    ),
  ).length;
  stats.push({ label: "Gone quiet", value: String(quiet) });
  return stats;
}

function atRiskCard(answer: Record<string, unknown>): HTMLElement {
  const deals = asList(answer.deals).map(asRecord);
  const scanned = asFiniteNumber(answer.deals_scanned) ?? 0;
  const floor =
    answer.truncated === true
      ? `The check stopped after ${scanned} deals, so there may be more.`
      : answer.coverage_withheld === true
        ? "Some deals could not be checked with your access."
        : undefined;
  const { card, body } = frame(
    "Relationships at risk",
    deals.length,
    [
      { label: "Deals checked", value: String(scanned) },
      {
        label: "At risk",
        value: String(deals.length),
        ...(deals.length > 0 ? { tone: "warning" as const } : {}),
      },
    ],
    floor,
  );
  if (deals.length === 0) {
    body.appendChild(
      el(
        "p",
        "intro",
        "No open deal rests on one relationship or has gone quiet.",
      ),
    );
    return card;
  }
  const list = el("ol");
  deals.forEach((deal, index) => {
    const name = asText(deal.name) || "Unnamed deal";
    const reasons = asList(deal.risks).map(asRecord).map(riskLine);
    list.appendChild(
      row(
        rankMarker(index + 1),
        name,
        reasons,
        "",
        `Catch me up on the deal "${name}" (${asText(deal.deal_id)}): who do we know there, and how do we widen it?`,
      ),
    );
  });
  card.appendChild(list);
  return card;
}

function riskLine(risk: Record<string, unknown>): HTMLElement {
  const names = asList(risk.contacts)
    .map((c) => asText(asRecord(c).name))
    .filter((n) => n !== "");
  const summary = asText(risk.summary);
  return reasonLine(
    names.length > 0 ? `${summary} (${names.join(", ")})` : summary,
  );
}

/** reasonLine is one reason an item is on the card, in the server's words,
 *  which are written in the workspace's own language. */
function reasonLine(words: string): HTMLElement {
  const line = el("p", "meta attention-reason");
  line.append(el("span", "attention-mark"), words);
  return line;
}

function commitmentsCard(answer: Record<string, unknown>): HTMLElement {
  const promises = asList(answer.commitments).map(asRecord);
  const state = (s: string) => promises.filter((p) => asText(p.state) === s);
  const overdue = state("overdue").length;
  const unowned = promises.filter((p) => asText(p.assignee_name) === "").length;
  const { card, body } = frame("Open promises", promises.length, [
    {
      label: "Overdue",
      value: String(overdue),
      ...(overdue > 0 ? { tone: "danger" as const } : {}),
    },
    { label: "Coming up", value: String(state("upcoming").length) },
    { label: "No date", value: String(state("undated").length) },
    {
      label: "Nobody owns",
      value: String(unowned),
      ...(unowned > 0 ? { tone: "warning" as const } : {}),
    },
  ]);
  if (promises.length === 0) {
    body.appendChild(
      el("p", "intro", "Every promise made to a customer is kept."),
    );
    return card;
  }
  const list = el("ol");
  for (const promise of promises) list.appendChild(promiseRow(promise));
  card.appendChild(list);
  return card;
}

function promiseRow(promise: Record<string, unknown>): HTMLElement {
  const subject = asText(promise.subject) || "A promise";
  const owner = asText(promise.assignee_name);
  const marker = avatar(
    owner === "" ? "?" : owner,
    asText(promise.assignee_id),
  );
  const reasons: HTMLElement[] = [];
  const status = el("p", "meta");
  status.append(dueBadge(promise), " ");
  status.append(owner === "" ? badge("Nobody owns this", "warning") : owner);
  reasons.push(status);
  const about = asList(promise.about)
    .map((a) => asText(asRecord(a).name))
    .filter((n) => n !== "");
  if (about.length > 0)
    reasons.push(el("p", "meta", `For ${about.join(", ")}`));
  const quote = asText(promise.quote);
  if (quote !== "") reasons.push(el("p", "meta attention-quote", `"${quote}"`));
  return row(
    marker,
    subject,
    reasons,
    "",
    `Catch me up on the promise "${subject}"${about.length > 0 ? ` to ${about.join(", ")}` : ""}: where does it stand, and what should happen next?`,
  );
}

function dueBadge(promise: Record<string, unknown>): HTMLElement {
  const due = asText(promise.due_at).slice(0, 10);
  switch (asText(promise.state)) {
    case "overdue": {
      const days = asFiniteNumber(promise.days_overdue) ?? 0;
      return badge(
        days === 0 ? "Late" : `${days} ${days === 1 ? "day" : "days"} late`,
        "danger",
      );
    }
    case "upcoming":
      return badge(due === "" ? "Coming up" : `Due ${due}`);
    default:
      return badge("No date");
  }
}
