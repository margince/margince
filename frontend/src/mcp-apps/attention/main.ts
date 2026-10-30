// The attention card: what read_attention answered, led by the figure that
// matters and a picture of its parts, then a ranked list a reader scans. One
// tool answers three questions, and the answer's shape says which: deals
// slipping this week, relationships at risk, or promises still open.
//
// Each row's "Catch me up" puts the follow-up question in the chat. The card
// calls no tool itself, so it declares no actions.

import { askAssistant } from "../actions";
import { badge } from "../badge";
import { el } from "../bridge";
import { hero, type Part, ring, segments, share } from "../hero";
import { avatar, button, panel, panelBody, viewRenderer } from "../parts";
import { refusalPanel } from "../summary";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import { compactMoney, money } from "../values";
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

/** The card's frame: a head with the count, and a body for the lead. */
function frame(
  title: string,
  count: number,
): { card: HTMLElement; body: HTMLElement } {
  const card = panel(title, { level: "h1", action: badge(String(count)) });
  const body = panelBody();
  card.appendChild(body);
  return { card, body };
}

type Row = Readonly<{
  marker: HTMLElement;
  name: string;
  reasons: HTMLElement[];
  figure?: string;
  bar?: HTMLElement;
  urgent?: boolean;
  question: string;
}>;

/** A ranked row: the marker, what it is and why, its figure, and the door. */
function row(r: Row): HTMLElement {
  const item = el("li", "attention-row");
  if (r.urgent === true) item.dataset.urgent = "";
  const text = el("div", "attention-text");
  const why = el("div", "attention-reasons");
  why.append(...r.reasons);
  text.append(el("p", "name attention-name", r.name), why);
  const end = el("div", "attention-end");
  if (r.figure !== undefined) {
    end.appendChild(el("span", "figure attention-figure", r.figure));
  }
  if (r.bar !== undefined) end.appendChild(r.bar);
  end.appendChild(
    button("Catch me up", "ghost", () => askAssistant(r.question)),
  );
  item.append(r.marker, text, end);
  return item;
}

function rankMarker(rank: number): HTMLElement {
  const node = el("span", "attention-rank figure", rank);
  node.setAttribute("aria-label", `Rank ${rank}`);
  return node;
}

/** reasonLine is one reason an item is on the card, in the server's words,
 *  which are written in the workspace's own language. */
function reasonLine(words: string): HTMLElement {
  const line = el("p", "meta attention-reason");
  line.append(el("span", "attention-mark"), words);
  return line;
}

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

function slippingCard(answer: Record<string, unknown>): HTMLElement {
  const deals = asList(answer.deals).map(asRecord);
  const { card, body } = frame("Slipping this week", deals.length);
  if (deals.length === 0) {
    body.appendChild(el("p", "intro", "No deal is slipping this week."));
    return card;
  }
  const stake = atStake(deals);
  const quiet = deals.filter((d) =>
    asList(d.evidence).some((e) =>
      ["deal.last_activity_at", "deal.created_at"].includes(
        asText(asRecord(e).source),
      ),
    ),
  ).length;
  body.appendChild(
    hero({
      kicker: "Pipeline at risk",
      figure: stake.figure,
      caption: stake.total === null ? "deals slipping" : "at stake",
      sentence: `${plural(deals.length, "deal is", "deals are")} slipping. ${plural(quiet, "has", "have")} gone quiet.`,
      ...(stake.parts.length > 1 ? { picture: segments(stake.parts) } : {}),
    }),
  );
  const list = el("ol", "attention-list");
  deals.forEach((deal, index) => {
    const name = asText(deal.name) || "Unnamed deal";
    const amount = asFiniteNumber(deal.amount_minor);
    const rank = asFiniteNumber(deal.rank) ?? index + 1;
    list.appendChild(
      row({
        marker: rankMarker(rank),
        name,
        reasons: asList(deal.evidence)
          .map(asRecord)
          .map((e) => reasonLine(asText(e.snippet))),
        ...(amount === null
          ? {}
          : { figure: money(deal.amount_minor, deal.currency) }),
        ...(amount === null || stake.total === null || stake.total <= 0
          ? {}
          : { bar: share(amount / stake.total, "flat") }),
        urgent: rank === 1,
        question: `Catch me up on the deal "${name}" (${asText(deal.deal_id)}): what happened and what should I do next?`,
      }),
    );
  });
  card.appendChild(list);
  return card;
}

/** What is at stake: one total, and each deal's part of it, when every amount
 *  is in one currency. Across currencies there is no total to state. */
function atStake(deals: Record<string, unknown>[]): {
  figure: string;
  total: number | null;
  parts: Part[];
} {
  const priced = deals.filter(
    (d) => asFiniteNumber(d.amount_minor) !== null && asText(d.currency) !== "",
  );
  const currencies = new Set(priced.map((d) => asText(d.currency)));
  const [only] = [...currencies];
  if (currencies.size !== 1 || only === undefined) {
    return { figure: String(deals.length), total: null, parts: [] };
  }
  const total = priced.reduce(
    (sum, d) => sum + (asFiniteNumber(d.amount_minor) ?? 0),
    0,
  );
  const parts: Part[] = priced.map((d, i) => ({
    label: asText(d.name) || "Unnamed deal",
    value: asFiniteNumber(d.amount_minor) ?? 0,
    shown: compactMoney(d.amount_minor, only),
    tone: i === 0 ? "warning" : "neutral",
  }));
  return { figure: compactMoney(total, only), total, parts };
}

function atRiskCard(answer: Record<string, unknown>): HTMLElement {
  const deals = asList(answer.deals).map(asRecord);
  const scanned = asFiniteNumber(answer.deals_scanned) ?? 0;
  const { card, body } = frame("Relationships at risk", deals.length);
  const floor =
    answer.truncated === true
      ? `The check stopped after ${scanned} deals, so there may be more.`
      : answer.coverage_withheld === true
        ? "Some deals could not be checked with your access."
        : undefined;
  body.appendChild(
    hero({
      kicker: "Relationship coverage",
      figure: String(deals.length),
      caption: `of ${plural(scanned, "deal", "deals")} at risk`,
      sentence:
        deals.length === 0
          ? "No open deal rests on one relationship or has gone quiet."
          : "These rest on one relationship, or the people on them have gone quiet.",
      picture: segments([
        { label: "At risk", value: deals.length, tone: "warning" },
        {
          label: "Covered",
          value: Math.max(0, scanned - deals.length),
          tone: "neutral",
        },
      ]),
    }),
  );
  if (floor !== undefined) body.appendChild(el("p", "readings-floor", floor));
  if (deals.length === 0) return card;
  const list = el("ol", "attention-list");
  deals.forEach((deal, index) => {
    const name = asText(deal.name) || "Unnamed deal";
    list.appendChild(
      row({
        marker: rankMarker(index + 1),
        name,
        reasons: asList(deal.risks).map(asRecord).map(riskLine),
        question: `Catch me up on the deal "${name}" (${asText(deal.deal_id)}): who do we know there, and how do we widen it?`,
      }),
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

function commitmentsCard(answer: Record<string, unknown>): HTMLElement {
  const promises = asList(answer.commitments).map(asRecord);
  const count = (s: string) =>
    promises.filter((p) => asText(p.state) === s).length;
  const overdue = count("overdue");
  const unowned = promises.filter((p) => asText(p.assignee_name) === "").length;
  const { card, body } = frame("Open promises", promises.length);
  if (promises.length === 0) {
    body.appendChild(
      el("p", "intro", "Every promise made to a customer is kept."),
    );
    return card;
  }
  const parts: Part[] = [
    { label: "Overdue", value: overdue, tone: "danger" },
    { label: "Coming up", value: count("upcoming"), tone: "accent" },
    { label: "No date", value: count("undated"), tone: "neutral" },
  ];
  body.appendChild(
    hero({
      kicker: "Promises to customers",
      figure: String(overdue),
      caption: "overdue",
      sentence:
        unowned === 0
          ? "Every promise has an owner."
          : `${plural(unowned, "promise has", "promises have")} nobody to keep it.`,
      beside: ring(parts, String(promises.length), "open"),
    }),
  );
  const list = el("ol", "attention-list");
  for (const promise of promises) list.appendChild(promiseRow(promise));
  card.appendChild(list);
  return card;
}

function promiseRow(promise: Record<string, unknown>): HTMLElement {
  const subject = asText(promise.subject) || "A promise";
  const owner = asText(promise.assignee_name);
  const status = el("p", "meta attention-status");
  status.append(dueBadge(promise), " ");
  status.append(owner === "" ? badge("Nobody owns this", "warning") : owner);
  const reasons = [status];
  const about = asList(promise.about)
    .map((a) => asText(asRecord(a).name))
    .filter((n) => n !== "");
  if (about.length > 0) {
    reasons.push(el("p", "meta", `For ${about.join(", ")}`));
  }
  const quote = asText(promise.quote);
  if (quote !== "") reasons.push(el("p", "attention-quote", quote));
  return row({
    marker: avatar(owner === "" ? "?" : owner, asText(promise.assignee_id)),
    name: subject,
    reasons,
    urgent: asText(promise.state) === "overdue",
    question: `Catch me up on the promise "${subject}"${about.length > 0 ? ` to ${about.join(", ")}` : ""}: where does it stand, and what should happen next?`,
  });
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
