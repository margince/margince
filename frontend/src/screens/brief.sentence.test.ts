import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import type { MessageKey } from "../i18n/en";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import {
  briefSentence,
  leadOf,
  sentenceParts,
  waitingRows,
} from "./brief.sentence";
import { itemTitle, rowHref } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The Brief's opening sentence.
//
// What these are about is the one property that makes a composed sentence
// honest: it may say nothing the rows below it do not already say. So the tests
// compare it against those rows rather than against a string.

/** The catalog's own lookup, with the interpolation the screen does. */
function t(key: MessageKey, values?: Record<string, string>): string {
  const raw = en[key];
  if (!values) {
    return raw;
  }
  return Object.entries(values).reduce<string>(
    (text, [name, value]) => text.replaceAll(`{${name}}`, value),
    raw,
  );
}

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "i1",
    source: "customer_waiting",
    level: 1,
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    consequence: "buyer_waits",
    actions: ["open"],
    dispositions: [],
    overdue: false,
    ...over,
  };
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-06-10T06:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    focus: {
      items: queue.slice(0, 6),
      total: queue.length,
      urgent_remaining: 0,
    },
    counts: [],
    reach: [],
    sources_unavailable: [],
    summary: { total: queue.length, urgent: 0, due: 0, lower_priority: 0 },
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: 0,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  };
}

describe("the opening sentence", () => {
  // A page that said "nothing needs you" over a read that never landed would
  // send a rep away believing their morning was clear. Absent is the honest
  // answer; a cheerful one is not.
  it("says nothing at all about a day it could not read", () => {
    expect(briefSentence(undefined, t, "en")).toBeNull();
    // Cast on purpose: `{}` is not a Worklist and is not meant to be. The case
    // is about a payload the function cannot read, which is the one shape a
    // typed fixture cannot express.
    expect(briefSentence({} as unknown as Worklist, t, "en")).toBeNull();
  });

  it("says the morning is clear only when the queue is genuinely empty", () => {
    const sentence = briefSentence(day([]), t, "en");
    expect(sentence?.key).toBe("brief.sentence.clear");
  });

  // THE PROPERTY THE WHOLE COMPOSITION RESTS ON. The sentence names the row the
  // section below draws first, through the same helper that row prints its own
  // title with — so the two cannot describe the same work differently.
  it("names the same lead the section below draws first, in its own words", () => {
    const first = item({ id: "a", title: "Aster Handel" });
    const rows = day([first, item({ id: "b", title: "Weber" })]);

    const sentence = briefSentence(rows, t, "en");
    expect(sentence?.values.lead).toBe(itemTitle(first, t, "en"));
    expect(leadOf(rows)?.id).toBe("a");
  });

  // The deck above answers approvals. A sentence opening with a decision the
  // section below deliberately did not draw would put the page's first words on
  // a row it does not show.
  it("names approvals in the same agenda as other work", () => {
    const rows = day([
      item({ id: "a", source: "approval", title: "Confirm the close date" }),
      item({ id: "b", title: "Aster Handel" }),
    ]);

    expect(leadOf(rows)?.id).toBe("a");
    expect(waitingRows(rows)).toHaveLength(2);
    const sentence = briefSentence(rows, t, "en");
    expect(
      // A key that renders no row title, for the case where no sentence was
      // composed at all: the assertion is about what the sentence does NOT
      // name, and a missing sentence names nothing either.
      t(sentence?.key ?? "brief.glance.intro", sentence?.values),
    ).toContain("Confirm the close date");
  });

  // A day whose only row IS a decision reads as clear HERE, because the deck is
  // where it is answered — not as an empty product.
  it("does not call an approval-only morning clear", () => {
    const sentence = briefSentence(
      day([item({ source: "approval" })]),
      t,
      "en",
    );
    expect(sentence?.key).toBe("brief.sentence.one");
  });

  // A ROW TITLE IS A CLAUSE, NOT A NOUN PHRASE. itemTitle returns whole
  // sentences, so a template that embedded one produced "Fang mit Die Nacht hat
  // das herausgesucht an" — which is not German, and no unit test caught it
  // because every assertion was about which key fired. The templates must place
  // the title after a separator rather than inside a phrase, in every language.
  it("never embeds the title inside a phrase, in any language", () => {
    for (const catalog of [en, de, vi]) {
      for (const key of [
        "brief.sentence.one",
        "brief.sentence.many",
      ] as const) {
        const template = catalog[key];
        // The hole is the last thing before a separator or the end — never
        // wrapped by words on both sides of the same clause.
        expect(template).toMatch(/[:—]\s*\{lead\}/);
      }
    }
  });

  // The remainder counts what is left AFTER the named row, over the rows this
  // page is answerable for — never over the raw queue.
  it("counts the rest over the rows the page shows", () => {
    const rows = day([
      item({ id: "a" }),
      item({ id: "b" }),
      item({ id: "c" }),
      item({ id: "d", source: "approval" }),
    ]);

    expect(briefSentence(rows, t, "en")?.values.rest).toBe("3");
  });

  it("names no remainder when the lead is the only row", () => {
    const sentence = briefSentence(day([item()]), t, "en");
    expect(sentence?.key).toMatch(/^brief\.sentence\.one/);
  });
});

// THE LEAD IS A WAY INTO THE ROW IT NAMES, not just words about it.
describe("the lead's own address", () => {
  it("carries the same address the row below it is linked by", () => {
    const lead = item({ id: "a", subject: { type: "deal", id: "d-1" } });
    const sentence = briefSentence(day([lead, item({ id: "b" })]), t, "en");

    expect(sentence?.leadHref).toBe(rowHref(lead));
    expect(sentence?.leadHref).toBeTruthy();
  });

  // A row that is neither a record nor a queue — a system condition fixed on a
  // settings screen the queue does not pretend to know — has no address, and
  // the sentence must say its words rather than link them nowhere.
  it("carries no address for a row that has none", () => {
    expect(briefSentence(day([item()]), t, "en")?.leadHref).toBeUndefined();
  });
});

// The sentence is translated with its holes INTACT and cut apart, because a
// string with the holes already filled has nowhere to put a link.
describe("sentenceParts", () => {
  it("keeps the template's own words and order around every hole", () => {
    expect(sentenceParts("First: {lead} Then {rest}.")).toEqual([
      { kind: "text", text: "First: " },
      { kind: "slot", name: "lead" },
      { kind: "text", text: " Then " },
      { kind: "slot", name: "rest" },
      { kind: "text", text: "." },
    ]);
  });

  // A hole at either end leaves no empty run beside it: an empty text part
  // would render an element with nothing in it, which a clamp counts as a line.
  it("leaves no empty run at either end", () => {
    expect(sentenceParts("{lead}")).toEqual([{ kind: "slot", name: "lead" }]);
  });

  // A template with nothing to fill is still a sentence. The clear morning's is
  // exactly that, and it must not come back empty.
  it("carries a template that has no holes at all", () => {
    expect(sentenceParts(en["brief.sentence.clear"])).toEqual([
      { kind: "text", text: en["brief.sentence.clear"] },
    ]);
  });

  // EVERY HOLE IN EVERY CATALOG IS ONE THIS SENTENCE CAN FILL. A translator who
  // spelled a hole the composer does not supply would print its own name on the
  // page, in that locale only.
  it("finds only holes the sentence supplies, in all three languages", () => {
    const supplied = new Set(["lead", "consequence", "rest"]);
    for (const catalog of [en, de, vi]) {
      for (const key of [
        "brief.sentence.one",
        "brief.sentence.many",
      ] as const) {
        for (const part of sentenceParts(catalog[key])) {
          if (part.kind === "slot") {
            expect(supplied.has(part.name)).toBe(true);
          }
        }
      }
    }
  });
});

it("does not call a partly read empty queue clear", () => {
  const partial = {
    ...day([]),
    sources_unavailable: [{ source: "task", reason: "failed" }],
  } satisfies Worklist;
  expect(briefSentence(partial, t, "en")).toBeNull();
});

it("counts actionable rows and keeps informational updates out of the headline", () => {
  const notice = item({
    source: "notice",
    title: "Northstar changed stage",
    level: 6,
    urgent: false,
  });
  const task = item({ source: "task", title: "Send the comparison" });
  const morning = {
    ...day([notice, task]),
    focus: { items: [task], total: 1, urgent_remaining: 0 },
  };
  expect(leadOf(morning)).toBe(task);
  expect(briefSentence(morning, t, "en")).toMatchObject({
    key: "brief.sentence.one",
    values: { rest: "0", lead: "Send the comparison" },
  });
  expect(
    briefSentence(
      { ...day([notice]), focus: { items: [], total: 0, urgent_remaining: 0 } },
      t,
      "en",
    )?.key,
  ).toBe("brief.sentence.clear");
  expect(waitingRows(day([{ ...notice, urgent: true }]))).toHaveLength(1);
  expect(waitingRows(day([{ ...notice, level: 0 }]))).toHaveLength(1);
});
