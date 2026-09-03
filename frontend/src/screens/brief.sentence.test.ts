import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import type { MessageKey } from "../i18n/en";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import {
  briefSentence,
  leadOf,
  leadRows,
  ledAlready,
  waitingRows,
} from "./brief.sentence";
import { overnightRow } from "./home.fixtures";
import { itemTitle } from "./worklist.copy";
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
    source: "waiting_customer",
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    actions: ["open"],
    dispositions: [],
    overdue: false,
    ...over,
  } as unknown as WorklistItem;
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-06-10T06:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    counts: [],
    reach: [],
    sources_unavailable: [],
    summary: { total: queue.length, urgent: 0 },
  } as unknown as Worklist;
}

describe("the opening sentence", () => {
  // A page that said "nothing needs you" over a read that never landed would
  // send a rep away believing their morning was clear. Absent is the honest
  // answer; a cheerful one is not.
  it("says nothing at all about a day it could not read", () => {
    expect(briefSentence(undefined, t, "en")).toBeNull();
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
  it("skips the decisions the deck answers when picking the lead", () => {
    const rows = day([
      item({ id: "a", source: "approval", title: "Confirm the close date" }),
      item({ id: "b", title: "Aster Handel" }),
    ]);

    expect(leadOf(rows)?.id).toBe("b");
    expect(waitingRows(rows)).toHaveLength(1);
    const sentence = briefSentence(rows, t, "en");
    expect(t(sentence?.key ?? "brief.eyebrow", sentence?.values)).not.toContain(
      "Confirm the close date",
    );
  });

  // A day whose only row IS a decision reads as clear HERE, because the deck is
  // where it is answered — not as an empty product.
  it("reads a deck-only morning as clear rather than as nothing at all", () => {
    const sentence = briefSentence(
      day([item({ source: "approval" })]),
      t,
      "en",
    );
    expect(sentence?.key).toBe("brief.sentence.clear");
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
        "brief.sentence.oneWithCost",
        "brief.sentence.many",
        "brief.sentence.manyWithCost",
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

    expect(briefSentence(rows, t, "en")?.values.rest).toBe("2");
  });

  it("names no remainder when the lead is the only row", () => {
    const sentence = briefSentence(day([item()]), t, "en");
    expect(sentence?.key).toMatch(/^brief\.sentence\.one/);
  });
});

// Which overnight suggestions the section BELOW must leave out.
//
// The Brief reads one brief run through two endpoints, so a suggestion that
// ranks into the lead is on the page twice unless the lower section skips it —
// once as a worklist row with its own controls, once as a card with its own.
// That is the same duplication the decisions deck exclusion exists to stop, one
// section further down.
describe("what already leads the page", () => {
  it("names the overnight rows Do next drew", () => {
    const today = day([
      overnightRow("bi-1", "d-1"),
      overnightRow("bi-2", "d-2"),
    ]);

    expect([...ledAlready(today)]).toEqual(["bi-1", "bi-2"]);
  });

  // The lead is a prefix of ONE order, so a suggestion ranked below it is not
  // on the page yet and the section below is the only place it appears.
  it("leaves out an overnight row that ranked past the lead", () => {
    const today = day([
      item({ id: "w1" }),
      item({ id: "w2" }),
      item({ id: "w3" }),
      overnightRow("bi-9", "d-9"),
    ]);

    expect(ledAlready(today).has("bi-9")).toBe(false);
    expect(leadRows(today).map((row) => row.id)).toEqual(["w1", "w2", "w3"]);
  });

  // Only overnight rows can collide: a waiting customer has no card below to be
  // drawn as, so naming it here would hide nothing and cost the reader a row.
  it("names nothing for a lead made of ordinary waiting work", () => {
    const today = day([item({ id: "w1" }), item({ id: "w2" })]);

    expect(ledAlready(today).size).toBe(0);
  });

  it("names nothing when the day could not be read", () => {
    expect(ledAlready(undefined).size).toBe(0);
  });
});
