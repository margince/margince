// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { entries, LETTER } from "../../scripts/lib/german-catalogs";

// German copy speaks to its reader as du, and two rules hold that.
//
//   1. No value addresses the reader formally: capitalised Sie / Ihr… IS that
//      address, and only the key families an outside reader opens are exempt,
//      by prefix. "Sie" opening a sentence as she or they is rephrased, never
//      waived, because the capital cannot be read off the page.
//   2. A du-pronoun is capitalised only where a sentence begins; mid-sentence
//      it is the formal register wearing the informal word.
//
// The mechanics of German copy are copy-style-de.test.ts's; both read the
// corpus scripts/lib/german-catalogs.ts derives.

// Longest first: the alternation is what gets reported, and "Ihr" would match
// the head of "Ihrem" and name the wrong word in the failure.
const FORMAL = new RegExp(
  `(?<![${LETTER}])(Ihnen|Ihres|Ihrem|Ihren|Ihrer|Ihre|Ihr|Sie)(?![${LETTER}])`,
  "g",
);

const DU = new RegExp(
  `(?<![${LETTER}])(Deines|Deinem|Deinen|Deiner|Deine|Dein|Dich|Dir|Du)(?![${LETTER}])`,
  "g",
);

// What can stand in front of a capital that is a capital only because a
// sentence starts there. `\}\s` is in the list because a placeholder's value is
// text this file cannot read: `{detail} Du kannst …` renders a sentence end
// where the catalog shows a brace.
const SENTENCE_OPENS = /(?:[.!?:]\s|\n|[„"(]|\}\s)$/;

function startsASentence(value: string, at: number): boolean {
  return at === 0 || SENTENCE_OPENS.test(value.slice(0, at));
}

// The key families a reader OUTSIDE this installation opens: a contact
// confirming consent, a stranger reading the privacy notice, a buyer in a deal
// room, a visitor choosing what may still be sent. They keep the formal
// address, because outbound mail to a contact is written in Sie by design
// (`confirmLines` in backend/internal/platform/mailcopy/catalog.go) and the
// consent question is published from the server in that wording, held by
// backend/gates/marketingquestion_test.go — a screen in du would ask one
// question and record another. Rule 2 still binds these values.
const READ_BY_AN_OUTSIDER: Readonly<Record<string, string>> = {
  "buyer.": "ein Käufer im Deal Room, der hier keinen Sitzplatz hat",
  "confirm.":
    "ein Kontakt, der die serverseitig gestellte Einwilligungsfrage beantwortet",
  "prefs.":
    "ein Besucher, der entscheidet, was diese Installation ihm senden darf",
  "privacynotice.": "ein Fremder, der liest, was über ihn gespeichert ist",
};

function readByAnOutsider(key: string): boolean {
  return Object.keys(READ_BY_AN_OUTSIDER).some((prefix) =>
    key.startsWith(prefix),
  );
}

describe("German copy speaks to its reader as du", () => {
  it("no value addresses the reader as Sie or Ihr", () => {
    const findings = entries().flatMap(([source, key, value]) =>
      readByAnOutsider(key)
        ? []
        : [...value.matchAll(FORMAL)].map((hit) => {
            const position = startsASentence(value, hit.index)
              ? "opening a sentence"
              : "mid-sentence";
            return `${source} ${key}: "${hit[0]}" ${position} in ${value}`;
          }),
    );
    expect(findings).toEqual([]);
  });

  it("every exempt prefix still names a key the catalogs carry", () => {
    const empty = Object.keys(READ_BY_AN_OUTSIDER).filter(
      (prefix) => !entries().some(([, key]) => key.startsWith(prefix)),
    );
    expect(empty).toEqual([]);
  });

  it("every exempt prefix says who reads it", () => {
    const silent = Object.entries(READ_BY_AN_OUTSIDER)
      .filter(([, why]) => why.trim() === "")
      .map(([prefix]) => prefix);
    expect(silent).toEqual([]);
  });

  it("a du-pronoun is capitalised only where a sentence begins", () => {
    const findings = entries().flatMap(([source, key, value]) =>
      [...value.matchAll(DU)]
        .filter((hit) => !startsASentence(value, hit.index))
        .map((hit) => `${source} ${key}: "${hit[0]}" mid-sentence in ${value}`),
    );
    expect(findings).toEqual([]);
  });
});
