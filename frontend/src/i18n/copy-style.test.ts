import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { en } from "./en";

// The mechanical half of docs/reference/ui-copy-style.md, held on the English
// catalog because every other locale is translated from it: a defect here
// becomes one per catalog. The rest of that page is the author's judgement.
// Each rule lists every offender as `key: "value"`, so one run names all the
// strings a rewrite has to touch.
const catalog: Record<string, string> = en;

// A placeholder names a parameter, not copy, and `{us}` or `{me}` would read as
// a word to the patterns below.
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
  return Object.entries(catalog)
    .filter(([key, value]) => !exempt(key) && pattern.test(text(value)))
    .map(([key, value]) => `${key}: ${JSON.stringify(value)}`);
}

// The privacy notice is the data controller addressing the data subject, and
// in law the controller speaks as "we".
const CONTROLLER_SPEAKS_PREFIX = "privacynotice.";

// The onboarding conversation, where an agent literally speaks in a bubble.
const AGENT_SPEAKS_PREFIX = "ob.conv.";

// A consent statement is the reader's sentence, not the product's.
const SPOKEN_BY_THE_READER = new Set<string>([
  "directSend.acknowledge",
  "book.consentWording",
  "prefs.wordingGeneric",
]);
const SPOKEN_BY_THE_READER_PREFIX = "prefs.wording.";

function spokenByTheReader(key: string): boolean {
  return (
    SPOKEN_BY_THE_READER.has(key) || key.startsWith(SPOKEN_BY_THE_READER_PREFIX)
  );
}

// Owned by the Vocabulary table's Never column on the style page; the last test
// fails a word here that the table does not name. The rest of that column is
// ordinary English in some sense ("account", "queue", "token"), so it stays
// the reviewer's.
const RETIRED_WORDS = [
  "workspace",
  "verdict",
  "spine",
  "backread",
  "deep read",
  "admission check",
  "carrier",
  "opportunity",
  "funnel",
  "promise",
  "Parts of this record",
  "deployment",
];

// Keys where a retired word keeps a sense the table does not retire.
const RETIRED_WORD_KEPT = new Map<string, string>([
  ["firstRun.platform.google", "Google Workspace is a product name"],
  ["company.lifecycle.opportunity", "a lifecycle stage, not the deal record"],
  ["signal.kind.new_opportunity", "a signal naming an opening, not a deal"],
  ["worklist.signal.opportunity", "a signal naming an opening, not a deal"],
]);

function retiredWordPattern(word: string): RegExp {
  const singularOrPlural = word.endsWith("y")
    ? `${word.slice(0, -1)}(?:y|ies)`
    : `${word}s?`;
  return new RegExp(`\\b${singularOrPlural}\\b`, "i");
}

function vocabularyNeverColumn(): string {
  const here = dirname(fileURLToPath(import.meta.url));
  const page = readFileSync(
    resolve(here, "..", "..", "..", "docs", "reference", "ui-copy-style.md"),
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

describe("en copy style", () => {
  it("uses no em dash or en dash", () => {
    expect(offenders(/[—–]/)).toEqual([]);
  });

  it("uses no exclamation mark", () => {
    expect(offenders(/!/)).toEqual([]);
  });

  it("spells apostrophes and quotes curly", () => {
    expect(offenders(/[A-Za-z]'[A-Za-z]|"/)).toEqual([]);
  });

  it("spells an ellipsis as the one character …", () => {
    expect(offenders(/\.\.\./)).toEqual([]);
  });

  // Edge whitespace is not checked: some values are fragments the code joins
  // around markup, and their leading or trailing space is load-bearing.
  it("has no double space", () => {
    expect(offenders(/ {2}/)).toEqual([]);
  });

  it("uses no Latin abbreviation", () => {
    expect(
      offenders(/\b(?:e\.g\.|i\.e\.|etc\.|vs\.)/i, { text: prose }),
    ).toEqual([]);
  });

  it("never says please", () => {
    expect(offenders(/\bplease\b/i, { text: prose })).toEqual([]);
  });

  it("uses no contraction", () => {
    const contraction =
      /\b(?:can|won|don|isn|aren|didn|doesn|couldn|wasn|hasn|haven)['’]t\b|\b(?:it|that|there|let)['’]s\b|\b(?:you|we|they)['’]re\b|\byou['’]ve\b/i;
    expect(offenders(contraction, { text: prose })).toEqual([]);
  });

  // Margince is software; it has no opinions, intentions or feelings to own.
  // Case-sensitive so the country code in "English (US)" is not read as "us".
  it("never speaks as we", () => {
    expect(
      offenders(/\b(?:We|we|Us|us|Our|our|Ours|ours)\b/, {
        text: prose,
        exempt: (key) =>
          key.startsWith(CONTROLLER_SPEAKS_PREFIX) || spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  // "me" and "my" stay legal: "Assign to me" and "My deals" name the reader's
  // own records in a control, not a voice. `\bI\b` also catches I’m and I’ve.
  it("says I only inside the onboarding conversation", () => {
    expect(
      offenders(/\bI\b|\b[Mm]yself\b/, {
        text: prose,
        exempt: (key) =>
          key.startsWith(AGENT_SPEAKS_PREFIX) || spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  it("spells American English", () => {
    const british =
      /\b(?:colour\w*|organis\w*|behaviour\w*|favourite\w*|centre[ds]?|licence[ds]?|cancell(?:ed|ing)|labell(?:ed|ing)|analyse[dr]?|analysing|catalogue[ds]?|recognis\w*|customis\w*|authoris\w*|prioritis\w*|summaris\w*|initialis\w*|optimis\w*|synchronis\w*|programmes?|grey(?:s|ed|ish)?|enrolments?|enrols?)\b/i;
    expect(offenders(british, { text: prose })).toEqual([]);
  });

  // A quoted label is copied from another product's screen and must match it.
  it("writes and, never an ampersand", () => {
    const unquoted = (value: string) => value.replace(/“[^”]*”/g, "");
    expect(offenders(/&/, { text: unquoted })).toEqual([]);
  });

  it("uses no retired word", () => {
    const found = RETIRED_WORDS.flatMap((word) =>
      offenders(retiredWordPattern(word), {
        text: prose,
        exempt: (key) => RETIRED_WORD_KEPT.has(key),
      }),
    );
    expect(found).toEqual([]);
    const stale = [...RETIRED_WORD_KEPT.keys()].filter(
      (key) =>
        !RETIRED_WORDS.some((word) =>
          retiredWordPattern(word).test(prose(catalog[key] ?? "")),
        ),
    );
    expect(stale).toEqual([]);
  });

  it("retires only words the style page's Vocabulary table retires", () => {
    const never = vocabularyNeverColumn();
    expect(never).not.toBe("");
    expect(
      RETIRED_WORDS.filter((word) => !retiredWordPattern(word).test(never)),
    ).toEqual([]);
  });
});
