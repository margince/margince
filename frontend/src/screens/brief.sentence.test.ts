import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import type { MessageKey } from "../i18n/en";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import { briefSentence, leadOf, waitingRows } from "./brief.sentence";
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
    expect(
      // A key that renders no row title, for the case where no sentence was
      // composed at all: the assertion is about what the sentence does NOT
      // name, and a missing sentence names nothing either.
      t(sentence?.key ?? "home.glance.intro", sentence?.values),
    ).not.toContain("Confirm the close date");
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
