// The forecast card: what read_forecast answered. A period's readings lead
// with the weighted figure, with each reading as a bar against the largest. A
// movement leads with the change and lists its causes.
//
// The input checks have no picture of their own, so they get a short verdict
// and leave the detail to the assistant. The card calls no tool itself.

import { askAssistant } from "../actions";
import { badge } from "../badge";
import { el } from "../bridge";
import { hero, share } from "../hero";
import { panel, panelBody, rowAction, viewRenderer } from "../parts";
import { refusalPanel } from "../summary";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type ToolCall,
  type Warning,
} from "../types";
import { compactMoney, day } from "../values";
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
  if ("weighted_minor" in answer) {
    root.appendChild(readingsCard(answer));
  } else if (Array.isArray(answer.buckets)) {
    root.appendChild(movementCard(answer));
  } else if ("readiness" in answer || "eligible_deals" in answer) {
    root.appendChild(checksCard(answer));
  } else {
    root.appendChild(refusalPanel("Nothing to show", null));
  }
}

type Bar = Readonly<{ label: string; value: number; shown: string }>;

/** bars draws each value as a bar against the largest, with its figure. */
function bars(rows: readonly Bar[]): HTMLElement {
  const most = Math.max(0, ...rows.map((r) => Math.abs(r.value)));
  const list = el("ul", "view-bars");
  for (const r of rows) {
    const item = el("li", "view-bar");
    item.append(
      el("span", "view-bar-label", r.label),
      share(most > 0 ? Math.abs(r.value) / most : 0, "flat"),
      el("span", "figure view-bar-figure", r.shown),
    );
    list.appendChild(item);
  }
  return list;
}

function readingsCard(answer: Record<string, unknown>): HTMLElement {
  const currency = answer.base_currency;
  const amount = (key: string) => asFiniteNumber(answer[key]) ?? 0;
  const shown = (key: string) => compactMoney(answer[key], currency);
  const period = `${day(answer.period_start)} to ${day(answer.period_end)}`;
  const card = panel("The forecast", { level: "h1", action: badge(period) });
  const body = panelBody();
  const eligible = asFiniteNumber(answer.eligible_count) ?? 0;
  const priced = asFiniteNumber(answer.priced_count) ?? 0;
  body.appendChild(
    hero({
      figure: shown("weighted_minor"),
      caption: "weighted",
      sentence: `${shown("won_minor")} is won and ${shown("open_minor")} is still open, across ${eligible} ${eligible === 1 ? "deal" : "deals"}.`,
      picture: bars([
        { label: "Won", value: amount("won_minor"), shown: shown("won_minor") },
        {
          label: "With evidence",
          value: amount("evidence_minor"),
          shown: shown("evidence_minor"),
        },
        {
          label: "Best case",
          value: amount("best_case_minor"),
          shown: shown("best_case_minor"),
        },
        {
          label: "Open",
          value: amount("open_minor"),
          shown: shown("open_minor"),
        },
      ]),
    }),
  );
  const note = asText(answer.coverage_note);
  if (note !== "") body.appendChild(el("p", "readings-floor", note));
  else if (priced < eligible) {
    body.appendChild(
      el(
        "p",
        "readings-floor",
        `${eligible - priced} of ${eligible} deals have no amount yet.`,
      ),
    );
  }
  const qualified = asRecord(answer.qualified);
  if ("weighted_minor" in qualified) {
    body.appendChild(
      el(
        "p",
        "readings-floor",
        `Counting only deals the customer still backs: ${compactMoney(qualified.weighted_minor, currency)} weighted.`,
      ),
    );
  }
  body.appendChild(
    rowAction("What moved it", () =>
      askAssistant(
        `What moved my forecast for ${period}? Read the movement between its last two snapshots and name the deals behind it.`,
      ),
    ),
  );
  card.appendChild(body);
  return card;
}

/** bucketLabel names a cause of movement as the app's waterfall does. */
function bucketLabel(name: string): string {
  switch (name) {
    case "new":
      return "New deals";
    case "pulled_in":
      return "Pulled into the period";
    case "pushed_out":
      return "Pushed out of the period";
    case "amount":
      return "Value changes";
    case "category":
      return "Forecast category changes";
    case "stage_weight":
      return "Stage weight changes";
    case "won":
      return "Won";
    case "lost":
      return "Lost";
    case "reopened_or_archived":
      return "Reopened or archived";
    case "fx":
      return "Exchange rate changes";
    case "definition":
      return "Definition changes";
    case "model":
      return "Model re-scoring";
    default:
      return "Other changes";
  }
}

/** signedPercent states a change as a share of where it started. */
function signedPercent(change: number, opening: number): string {
  const rounded = Math.round((change / opening) * 100);
  return rounded > 0 ? `+${rounded}%` : `${rounded}%`;
}

/**
 * movementCard draws the change between two snapshots. The answer names no
 * currency, so the card states shares of the opening figure, never money.
 */
function movementCard(answer: Record<string, unknown>): HTMLElement {
  const opening = asFiniteNumber(answer.opening_minor) ?? 0;
  const closing = asFiniteNumber(answer.closing_minor) ?? 0;
  const buckets = asList(answer.buckets)
    .map(asRecord)
    .map((b) => ({
      label: bucketLabel(asText(b.name)),
      amount: asFiniteNumber(b.amount_minor) ?? 0,
      deals: asFiniteNumber(b.deal_count) ?? 0,
    }))
    .filter((b) => b.amount !== 0 || b.deals > 0)
    .sort((a, b) => Math.abs(b.amount) - Math.abs(a.amount));
  const moved = asList(answer.deals).length;
  const card = panel("What moved the forecast", {
    level: "h1",
    action: badge(`${moved} ${moved === 1 ? "deal" : "deals"}`),
  });
  const body = panelBody();
  const [largest] = buckets;
  body.appendChild(
    hero({
      figure:
        opening === 0
          ? String(moved)
          : signedPercent(closing - opening, opening),
      caption: opening === 0 ? "deals moved" : "since the opening snapshot",
      sentence:
        largest === undefined
          ? "Nothing moved between the two snapshots."
          : `${largest.label} moved it most.`,
      ...(buckets.length > 0
        ? {
            picture: bars(
              buckets.map((b) => ({
                label: `${b.label} (${b.deals} ${b.deals === 1 ? "deal" : "deals"})`,
                value: b.amount,
                shown:
                  opening === 0
                    ? String(b.deals)
                    : signedPercent(b.amount, opening),
              })),
            ),
          }
        : {}),
    }),
  );
  if (moved > 0) {
    body.appendChild(
      rowAction("Name the deals", () =>
        askAssistant(
          "Name the deals that moved my forecast between these two snapshots, biggest change first, with a link to each.",
        ),
      ),
    );
  }
  card.appendChild(body);
  return card;
}

/** checksCard states whether the forecast's inputs can be trusted. */
function checksCard(answer: Record<string, unknown>): HTMLElement {
  const verdict = readinessWords(asText(answer.readiness));
  const card = panel("The forecast's inputs", {
    level: "h1",
    action: badge(verdict.label, verdict.tone),
  });
  const body = panelBody();
  const deals = asFiniteNumber(answer.eligible_deals);
  body.appendChild(
    el(
      "p",
      "intro",
      deals === null
        ? verdict.sentence
        : `${verdict.sentence} The check read ${deals} ${deals === 1 ? "deal" : "deals"}.`,
    ),
  );
  card.appendChild(body);
  return card;
}

function readinessWords(readiness: string): {
  label: string;
  tone: "success" | "warning" | "danger" | "default";
  sentence: string;
} {
  switch (readiness) {
    case "ready":
      return {
        label: "Ready",
        tone: "success",
        sentence: "Every check passed.",
      };
    case "ready_with_exceptions":
      return {
        label: "Ready, with exceptions",
        tone: "warning",
        sentence: "The checks passed, with a few deals to look at.",
      };
    case "needs_review":
      return {
        label: "Needs review",
        tone: "danger",
        sentence: "Some inputs need a look before the forecast is called.",
      };
    default:
      return {
        label: "Not checked",
        tone: "default",
        sentence: "The checks could not finish, so nobody could look yet.",
      };
  }
}
