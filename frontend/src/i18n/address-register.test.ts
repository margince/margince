// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesMatching } from "../../scripts/lib/source-tree";
import { de } from "./de";

// German copy speaks to its reader as du, and three rules hold that.
//
//   1. No value addresses the reader formally. Capitalised Sie / Ihr… IS that
//      address, with one position German orthography leaves ambiguous: "Sie"
//      opening a sentence is also she and they. That position alone is
//      waivable, with a reason naming what the pronoun stands for, and the key
//      families an outside reader opens are exempt by prefix.
//   2. A du-pronoun is capitalised only where a sentence begins; mid-sentence
//      it is the formal register wearing the informal word.
//   3. A long dash does not belong in German copy, where a comma, a period, a
//      colon or parentheses carries the break. A ratchet, not a bar.
//
// The corpus is DERIVED from every German catalog a bundler ships, core and
// extension: a gate naming one file reads a smaller tree and still says PASS.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");
const extensionsDir = resolve(repoRoot, "extensions");

type Catalog = Readonly<Record<string, string>>;
type GermanCopy = Readonly<{ source: string; catalog: Catalog }>;

// The letters a German word is made of. `\b` answers for none of the umlauts,
// so a boundary written with it would match inside a word that carries one.
const LETTER = "A-Za-zÄÖÜäöüß";

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

// A waiver for the one position where a capital cannot be read off the page.
// `word` scopes it: a key whose sentence opens with "Sie" the pronoun does not
// thereby get to carry "Ihre" the address further along.
type Waiver = Readonly<{ word: string; why: string }>;

const OPENS_WITH_A_PRONOUN: Readonly<Record<string, Waiver>> = {
  "aiAdmin.coverage": {
    word: "Sie",
    why: "die Zahlen zählen nicht jeden Lauf",
  },
  "aicalls.withheld": { word: "Sie", why: "die Aufrufspur verzeichnet" },
  "aiProviderKeys.absentHint": {
    word: "Sie",
    why: "die Zugangsdaten können auch über die Umgebung ankommen",
  },
  "analytics.share.snapshotHelp": {
    word: "Sie",
    why: "die eingefrorenen Zahlen ändern sich nicht",
  },
  "common.gatewayUnavailable": {
    word: "Sie",
    why: "die Anfrage läuft möglicherweise noch",
  },
  "consent.offline": { word: "Sie", why: "die Verbindung bleibt bestehen" },
  "contact.intro.verdictDirectYou": {
    word: "Ihr",
    why: "ihr beide, du und der Kontakt",
  },
  "firstRun.google.helpStep2": {
    word: "Sie",
    why: "die beiden Bereiche gehören in eine Zustimmung",
  },
  "forecast.landing.caveat.call_below_actual": {
    word: "Sie",
    why: "die Einschätzung wird wie erfasst gezeigt",
  },
  "ob.conv.connect.appUnusableCard": {
    word: "Sie",
    why: "die App braucht einen Admin",
  },
  "plan.saveRefused_one": {
    word: "Sie",
    why: "die Zusage ist weiterhin angehakt",
  },
  "plan.saveRefused_other": {
    word: "Sie",
    why: "die Zusagen sind weiterhin angehakt",
  },
  "provider.profile.emptyBody": {
    word: "Sie",
    why: "die Abfrage kostet Credits",
  },
  "provider.profile.submissionUnknown": {
    word: "Sie",
    why: "die Abfrage kann berechnet worden sein",
  },
  "restricted.pin.idMalformed": {
    word: "Sie",
    why: "die Datensatz-ID besteht aus Hexadezimalzeichen",
  },
  "restricted.sub": {
    word: "Sie",
    why: "die Korrespondenz ist eingeschränkt",
  },
  "sched.withdrawBody": {
    word: "Sie",
    why: "die zurückgezogene Nachricht müsste neu verfasst werden",
  },
  "sendPermission.reason.ambiguous": {
    word: "Sie",
    why: "die Datensätze zusammenzuführen ist die Lösung",
  },
  "sendPermission.reason.bounced": {
    word: "Sie",
    why: "die Adresse zu korrigieren ist die Lösung",
  },
  "settings.voice.worksEmails": {
    word: "Sie",
    why: "die gesendeten E-Mails zeigen den Schreibstil",
  },
  "settings.voice.worksNot": {
    word: "Sie",
    why: "fremde Texte würden die falsche Stimme beibringen",
  },
  "setup.baseCurrencyHint": {
    word: "Sie",
    why: "die Währung lässt sich noch ändern",
  },
  "signInMethods.passwordReason": {
    word: "Sie",
    why: "die Passwort-Anmeldung hält eine Installation zugänglich",
  },
  "users.deactivateAgentConfirmBody": {
    word: "Sie",
    why: "die Agent-Identität meldet sich nirgends an",
  },
};

// The key families a reader OUTSIDE this installation opens: a contact
// confirming consent, a stranger reading the privacy notice, a buyer in a deal
// room, a visitor choosing what may still be sent. They keep the formal
// address, because outbound mail to a contact is written in Sie by design
// (`confirmLines` in backend/internal/platform/mailcopy/catalog.go) and the
// consent question is published from the server in that wording, held by
// backend/gates/marketingquestion_test.go — a screen in du would ask one
// question and record another. Rules 2 and 3 still bind these values.
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

// The long dashes the German catalog still carries. It may fall and never rise:
// whoever removes one lowers this number in the same change, and nothing is
// allowed to add one.
const LONG_DASHES_PINNED = 371;

function readJsonCatalog(path: string): Catalog {
  const parsed: unknown = JSON.parse(readFileSync(path, "utf8"));
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error(`${path} is not a JSON object of message keys`);
  }
  const catalog: Record<string, string> = {};
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value !== "string") {
      throw new Error(`${path}: ${key} does not hold a string`);
    }
    catalog[key] = value;
  }
  return catalog;
}

// Every unit's German catalog, found the way a bundler finds one: a file named
// de.json inside a frontend layer, at whatever depth the unit put it.
function unitCopy(): GermanCopy[] {
  return extensionLayers(extensionsDir)
    .flatMap((layer) => filesMatching(layer, /^de\.json$/))
    .sort()
    .map((path) => ({
      source: relative(repoRoot, path),
      catalog: readJsonCatalog(path),
    }));
}

const GERMAN: readonly GermanCopy[] = [
  { source: "frontend/src/i18n/de.ts", catalog: de },
  ...unitCopy(),
];

function entries(): Array<readonly [string, string, string]> {
  return GERMAN.flatMap(({ source, catalog }) =>
    Object.entries(catalog).map(
      ([key, value]) => [source, key, value] as const,
    ),
  );
}

describe("German copy speaks to its reader as du", () => {
  it("no value addresses the reader as Sie or Ihr", () => {
    const findings = entries().flatMap(([source, key, value]) =>
      readByAnOutsider(key)
        ? []
        : [...value.matchAll(FORMAL)].flatMap((hit) => {
            const opening = startsASentence(value, hit.index);
            const waiver = OPENS_WITH_A_PRONOUN[key];
            if (opening && waiver?.word === hit[0]) return [];
            const position = opening ? "opening a sentence" : "mid-sentence";
            return [`${source} ${key}: "${hit[0]}" ${position} in ${value}`];
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

  it("every waived key still opens a sentence with the word it is waived for", () => {
    const spent = Object.entries(OPENS_WITH_A_PRONOUN)
      .filter(([key, waiver]) => {
        const carried = GERMAN.map(({ catalog }) => catalog[key]).find(
          (value) => value !== undefined,
        );
        if (carried === undefined) return true;
        return ![...carried.matchAll(FORMAL)].some(
          (hit) =>
            hit[0] === waiver.word && startsASentence(carried, hit.index),
        );
      })
      .map(
        ([key, waiver]) =>
          `${key} is waived for "${waiver.word}" it no longer opens a sentence with`,
      );
    expect(spent).toEqual([]);
  });

  it("every waiver says who or what it is waived for", () => {
    const silent = [
      ...Object.entries(OPENS_WITH_A_PRONOUN).map(
        ([key, waiver]) => [key, waiver.why] as const,
      ),
      ...Object.entries(READ_BY_AN_OUTSIDER),
    ]
      .filter(([, why]) => why.trim() === "")
      .map(([key]) => key);
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

  it("carries no more long dashes than it was pinned at", () => {
    const dashed = entries().filter(([, , value]) => /[–—]/.test(value));
    expect(dashed.length).toBeLessThanOrEqual(LONG_DASHES_PINNED);
  });
});
