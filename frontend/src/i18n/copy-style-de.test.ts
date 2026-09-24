// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { entries, GERMAN, LETTER } from "../../scripts/lib/german-catalogs";

// The mechanical half of docs/reference/ui-copy-style-de.md, held on every
// German catalog a bundler ships. Address is address-register.test.ts's. Each
// rule lists every offender as `source key: "value"`, so one run names all the
// strings a rewrite has to touch.

// A placeholder names a parameter, not copy, and `{uns}` or `{ich}` would read
// as a word to the patterns below.
function prose(value: string): string {
  return value.replace(/\{[^}]*\}/g, "");
}

function offenders(
  pattern: RegExp,
  options: {
    text?: (value: string) => string;
    exempt?: (key: string) => boolean;
  } = {},
): string[] {
  const { text = (value: string) => value, exempt = () => false } = options;
  return entries()
    .filter(([, key, value]) => !exempt(key) && pattern.test(text(value)))
    .map(
      ([source, key, value]) => `${source} ${key}: ${JSON.stringify(value)}`,
    );
}

function word(alternation: string, flags = ""): RegExp {
  return new RegExp(`(?<![${LETTER}])(?:${alternation})(?![${LETTER}])`, flags);
}

// The onboarding conversation, where an agent literally speaks in a bubble.
const AGENT_SPEAKS_PREFIX = "ob.conv.";

// The privacy notice is the data controller addressing the data subject, and
// in law the controller speaks as "wir".
const CONTROLLER_SPEAKS_PREFIX = "privacynotice.";

// A consent statement is the reader's sentence, not the product's. The prefix
// takes both `prefs.wording.*` and `prefs.wordingGeneric`.
const SPOKEN_BY_THE_READER = new Set<string>([
  "directSend.acknowledge",
  "book.consentWording",
]);
const SPOKEN_BY_THE_READER_PREFIX = "prefs.wording";

function spokenByTheReader(key: string): boolean {
  return (
    SPOKEN_BY_THE_READER.has(key) || key.startsWith(SPOKEN_BY_THE_READER_PREFIX)
  );
}

// Owned by the Vocabulary table's Never column on the German style page; the
// last test fails a word here that the table does not name. Whole words and
// case-sensitive, so the noun "Versprechen" is retired and a closed compound
// such as "Nutzerkonto" is not.
const RETIRED_WORDS = [
  "Firma",
  "Firmen",
  "Workspace",
  "Arbeitsbereich",
  "Account",
  "Accounts",
  "Mandant",
  "Opportunity",
  "Opportunities",
  "Verkaufschance",
  "Verkaufschancen",
  "Trichter",
  "Funnel",
  "Prognose",
  "Prognosen",
  "Arbeitsliste",
  "Briefing",
  "Tagesbericht",
  "genehmigen",
  "genehmigt",
  "Genehmigung",
  "Genehmigungen",
  "Urteil",
  "Urteile",
  "Verdikt",
  "Versprechen",
  "Owner",
  "Besitzer",
  "Aufbewahrungssperre",
  "Einkaufskomitee",
  "Kaufgremium",
  "Deployment",
  "Instanz",
  "Administrator",
  "Administratoren",
  "Verwalter",
  "Nutzer",
  "Nutzern",
  "Benutzer",
  "Benutzern",
  "Anwender",
  "Anwendern",
  "Mitarbeiter",
  "Mitarbeitern",
  "Vertriebsmitarbeiter",
  "Deal-Room",
  "Spine",
  "Backread",
  "Deep Read",
  "Zulassungsprüfung",
  "Audit-Trail",
  "Prüfprotokoll",
  "AI",
  "einloggen",
  "eingeloggt",
  "Nochmal",
  "nochmal",
  "uploaden",
  "downloaden",
  "Email",
  "Emails",
  "eMail",
  "Mailbox",
  "Meeting",
  "Meetings",
  "Task",
  "Tasks",
  "Schlagwort",
  "Report",
  "Reports",
  "Allowance",
  "Seat",
  "Seats",
  "Privatsphäre",
  "GDPR",
];

// Keys where a retired word keeps a sense the table does not retire.
const RETIRED_WORD_KEPT = new Map<string, string>([
  ["firstRun.platform.google", "Google Workspace is a product name"],
]);

function retiredWordPattern(retired: string): RegExp {
  return word(retired.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
}

function vocabularyNeverColumn(): string {
  const here = dirname(fileURLToPath(import.meta.url));
  const page = readFileSync(
    resolve(here, "..", "..", "..", "docs", "reference", "ui-copy-style-de.md"),
    "utf8",
  );
  const section = page.split("\n## Vocabulary\n")[1]?.split("\n## ")[0] ?? "";
  return section
    .split("\n")
    .filter((line) => line.startsWith("|") && !line.startsWith("|---"))
    .slice(1)
    .map((row) => row.split("|")[3] ?? "")
    .join("\n");
}

describe("de copy style", () => {
  it("uses no en dash, em dash or double hyphen", () => {
    expect(offenders(/[–—]|--/)).toEqual([]);
  });

  it("uses no straight double quote", () => {
    expect(offenders(/"/)).toEqual([]);
  });

  it("uses no straight apostrophe between letters", () => {
    expect(offenders(new RegExp(`[${LETTER}]'[${LETTER}]`))).toEqual([]);
  });

  // A „…“ pair is removed first, so what remains is a mark used some other
  // way: an English pair, guillemets, or an opening „ nothing closes.
  it("quotes with „…“ only", () => {
    const unpaired = (value: string) => value.replace(/„[^„“]*“/g, "");
    expect(offenders(/[„“”»«]/, { text: unpaired })).toEqual([]);
  });

  it("spells an ellipsis as the one character …", () => {
    expect(offenders(/\.\.\./)).toEqual([]);
  });

  // Edge whitespace is not checked: some values are fragments the code joins
  // around markup, and their leading or trailing space is load-bearing.
  it("has no double space", () => {
    expect(offenders(/ {2}/)).toEqual([]);
  });

  it("uses no exclamation mark", () => {
    expect(offenders(/!/)).toEqual([]);
  });

  it("never says bitte", () => {
    expect(offenders(word("bitte", "i"), { text: prose })).toEqual([]);
  });

  // Case-sensitive past the first letter, so the certificate authority "CA."
  // closing a sentence is not read as "ca.".
  it("uses no abbreviation", () => {
    const abbreviation = new RegExp(
      `(?<![${LETTER}])(?:[zZ]\\.\\s?B\\.|[dD]\\.\\s?h\\.|[uU]sw\\.|[uU]\\.\\s?a\\.|[gG]gf\\.|[bB]zgl\\.|[bB]zw\\.|[iI]nkl\\.|[cC]a\\.)`,
    );
    expect(offenders(abbreviation, { text: prose })).toEqual([]);
  });

  it("uses no apostrophe contraction", () => {
    expect(
      offenders(new RegExp(`[${LETTER}]['’]s(?![${LETTER}])`), { text: prose }),
    ).toEqual([]);
  });

  // Read on the raw value: `{pct}%` is a number the formatter supplies, glued
  // to its sign exactly as a literal digit would be.
  it("sets a no-break space before a percent sign", () => {
    expect(offenders(/[\d}] ?%/)).toEqual([]);
  });

  it("writes no gender asterisk, colon, gap, slash or Binnen-I", () => {
    expect(offenders(/[*:_/-]innen|[a-zäöüß]Innen/, { text: prose })).toEqual(
      [],
    );
  });

  // "mir" and "Meine" stay legal: "Mir zuweisen" and "Meine Deals" name the
  // reader in a control, not a voice.
  it("says ich only inside the onboarding conversation", () => {
    expect(
      offenders(word("ich|Ich|mich|Mich"), {
        text: prose,
        exempt: (key) =>
          key.startsWith(AGENT_SPEAKS_PREFIX) || spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  it("never speaks as wir", () => {
    expect(
      offenders(word(`wir|Wir|uns|Uns|unser[${LETTER}]*|Unser[${LETTER}]*`), {
        text: prose,
        exempt: (key) =>
          key.startsWith(CONTROLLER_SPEAKS_PREFIX) || spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  it("uses no retired word", () => {
    const found = RETIRED_WORDS.flatMap((retired) =>
      offenders(retiredWordPattern(retired), {
        text: prose,
        exempt: (key) => RETIRED_WORD_KEPT.has(key),
      }),
    );
    expect(found).toEqual([]);
    const stale = [...RETIRED_WORD_KEPT.keys()].filter((key) => {
      const carried = GERMAN.map(({ catalog }) => catalog[key]).find(
        (value) => value !== undefined,
      );
      return !RETIRED_WORDS.some((retired) =>
        retiredWordPattern(retired).test(prose(carried ?? "")),
      );
    });
    expect(stale).toEqual([]);
  });

  it("retires only words the German style page's Vocabulary table retires", () => {
    const never = vocabularyNeverColumn();
    expect(never).not.toBe("");
    expect(
      RETIRED_WORDS.filter(
        (retired) => !retiredWordPattern(retired).test(never),
      ),
    ).toEqual([]);
  });
});
