// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The mechanical half of docs/reference/ui-copy-style-de.md, held on every
// German catalog a bundler ships. Address is the address-register rule's.
// Each check lists every offender, so one run names all the strings a rewrite
// has to touch.
//
// The core catalog is read from the tree's de.ts. A unit ships its copy as
// de.json beside en.json, found in the git listing.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import type ts from "typescript";
import { LETTER, readByAnOutsider } from "../../scripts/lib/german-catalogs";
import {
  catalogDeclaredIn,
  catalogEntries,
  type LocaleCopy,
  listedUnitCopy,
} from "../../scripts/lib/locale-catalogs";
import { en } from "../../src/i18n/en";
import { type CensusRule, repoRoot, srcDir } from "../tree";

export const coreCatalog = join(srcDir, "i18n", "de.ts");

/** Every German catalog: the core one parsed from the tree, then each unit's. */
export function germanCopies(core: ts.SourceFile): LocaleCopy[] {
  return [
    {
      source: "frontend/src/i18n/de.ts",
      catalog: catalogDeclaredIn(core, "de"),
      english: en,
    },
    ...listedUnitCopy("de"),
  ];
}

// A placeholder names a parameter, not copy, and `{uns}` or `{ich}` would read
// as a word to the patterns below.
function prose(value: string): string {
  return value.replace(/\{[^}]*\}/g, "");
}

function word(alternation: string, flags = ""): RegExp {
  return new RegExp(`(?<![${LETTER}])(?:${alternation})(?![${LETTER}])`, flags);
}

// The onboarding conversation, where an agent literally speaks in a bubble.
const AGENT_SPEAKS_PREFIX = "ob.conv.";

// The privacy notice is the data controller addressing the data subject, and
// in law the controller speaks as "wir".
const CONTROLLER_SPEAKS_PREFIX = "privacynotice.";

// A consent statement, and a promise the reader makes by pressing it, are the
// reader's sentences, not the product's. The prefix takes both
// `prefs.wording.*` and `prefs.wordingGeneric`.
const SPOKEN_BY_THE_READER = new Set<string>([
  "commitment.takeOver",
  "directSend.acknowledge",
  "directSend.correctionAcknowledge",
  "book.consentWording",
  "worklist.ready.confirm",
]);
const SPOKEN_BY_THE_READER_PREFIX = "prefs.wording";

function spokenByTheReader(key: string): boolean {
  return (
    SPOKEN_BY_THE_READER.has(key) || key.startsWith(SPOKEN_BY_THE_READER_PREFIX)
  );
}

// Owned by the Vocabulary table's Never column on the German style page. That
// column sets each of these words in code format, and the last test holds the
// two equal.
// Whole words and case-sensitive, so the noun "Versprechen" is retired and a
// closed compound such as "Nutzerkonto" is not.
const RETIRED_WORDS = [
  "Firma",
  "Firmen",
  "Workspace",
  "Arbeitsbereich",
  "Account",
  "Accounts",
  "Mandant",
  "Ansprechpartner",
  "Opportunity",
  "Opportunities",
  "Verkaufschance",
  "Verkaufschancen",
  "Trichter",
  "Funnel",
  "Forecast",
  "Forecasts",
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
  "Inhaber",
  "Aufbewahrungssperre",
  "Einkaufskomitee",
  "Kaufgremium",
  "Deployment",
  "Instanz",
  "Administrator",
  "Administratoren",
  "Verwalter",
  "Systemadministrator",
  "Kollege",
  "Kollegin",
  "Kollegen",
  "Nutzer",
  "Nutzern",
  "Benutzer",
  "Benutzern",
  "Benutzende",
  "Anwender",
  "Anwenders",
  "Anwendern",
  "Mitarbeiter",
  "Mitarbeitern",
  "Vertriebsmitarbeiter",
  "Empfänger",
  "Empfängern",
  "Absender",
  "Absenders",
  "Absendern",
  "Autor",
  "Autors",
  "Autoren",
  "Eigentümer",
  "Eigentümers",
  "Eigentümern",
  "Teilnehmer",
  "Teilnehmers",
  "Teilnehmern",
  "Käufer",
  "Käufers",
  "Käufern",
  "Verkäufer",
  "Verkäufers",
  "Verkäufern",
  "Ersteller",
  "Erstellers",
  "Erstellern",
  "Leser",
  "Lesers",
  "Lesern",
  "Deal-Room",
  "Spine",
  "Backread",
  "Gastgeber",
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
  "Gegenbuchung",
  "erfolgt",
  "seitens",
  "Seitens",
  "hinsichtlich",
  "Hinsichtlich",
  "bezüglich",
  "Bezüglich",
  "mittels",
  "Mittels",
  "zwecks",
  "Zwecks",
  "Durchführung",
  "Vornahme",
  "Inanspruchnahme",
];

// Keys where a retired word keeps a sense the table does not retire.
const RETIRED_WORD_KEPT = new Map<string, string>([
  ["firstRun.platform.google", "Google Workspace is a product name"],
  ["oauthApp.google.for", "Google Workspace is a product name"],
  ["oauthApp.tenant", "Microsoft Entra labels the directory tenant Mandant"],
  ["aiProviders.name.geminiVertex", "Vertex AI is Google's product name"],
  [
    "aiProviderSettings.service.googleAiStudio",
    "Google AI Studio is Google's product name",
  ],
  [
    "aiProviderKeys.vertexRoleHint",
    "Vertex AI User is Google's name for the role",
  ],
  [
    "aiProviderKeys.reason.permissionDeniedVertex",
    "Vertex AI and its Vertex AI User role are Google's names",
  ],
]);

// Read on the raw value, so a placeholder followed by a space opens a sentence
// as it does for the address-register rule. The verb takes the family's address.
function recoveryBitte(verbs: string): RegExp {
  return new RegExp(
    `(?:^|[.?!:]\\s|\\}\\s)Bitte (?:${verbs})(?![${LETTER}])`,
    "g",
  );
}
const RECOVERY_BITTE_DU = recoveryBitte("versuche|lade|wende");
const RECOVERY_BITTE_SIE = recoveryBitte("versuchen Sie|laden Sie|wenden Sie");

function misplacedBitte(recovery: RegExp): (value: string) => boolean {
  return (value) => {
    const said = value.match(word("bitte", "gi"))?.length ?? 0;
    if (said === 0) return false;
    const placed = value.match(recovery)?.length ?? 0;
    return said > 1 || placed < said || !/\.(?:\s|$)/.test(value);
  };
}

function retiredWordPattern(retired: string): RegExp {
  return word(retired.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
}

// The words the Never column marks as gated. A plain word there is the
// reviewer's, held only in the sense its row gives.
function gatedNeverWords(neverColumn: string): string[] {
  return [...neverColumn.matchAll(/`([^`]+)`/g)].map(([, gated]) => gated);
}

export function vocabularyNeverColumn(): string {
  const page = readFileSync(
    join(repoRoot, "docs", "reference", "ui-copy-style-de.md"),
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

const ABBREVIATION = new RegExp(
  `(?<![${LETTER}])(?:[zZ]\\.\\s?B\\.|[dD]\\.\\s?h\\.|[uU]sw\\.|[uU]\\.\\s?a\\.|[gG]gf\\.|[bB]zgl\\.|[bB]zw\\.|[iI]nkl\\.|[cC]a\\.|Tg\\.)`,
);

/** Every finding over the given catalogs, each led by the check it breaks. */
export function styleFindings(
  copies: readonly LocaleCopy[],
  neverColumn: string = vocabularyNeverColumn(),
): string[] {
  const entries = catalogEntries(copies);
  const offenders = (
    check: string,
    breaks: RegExp | ((value: string) => boolean),
    options: {
      text?: (value: string) => string;
      exempt?: (key: string) => boolean;
    } = {},
  ): string[] => {
    const { text = (value: string) => value, exempt = () => false } = options;
    const broken =
      breaks instanceof RegExp ? (value: string) => breaks.test(value) : breaks;
    return entries
      .filter(([, key, value]) => !exempt(key) && broken(text(value)))
      .map(
        ([source, key, value]) =>
          `${check}: ${source} ${key}: ${JSON.stringify(value)}`,
      );
  };
  // A „…“ pair is removed first, so what remains is a mark used some other
  // way: an English pair, guillemets, or an opening „ nothing closes.
  const unpaired = (value: string) => value.replace(/„[^„“]*“/g, "");
  const stale = [...RETIRED_WORD_KEPT.keys()].filter((key) => {
    const carried = copies
      .map(({ catalog }) => catalog[key])
      .find((value) => value !== undefined);
    return !RETIRED_WORDS.some((retired) =>
      retiredWordPattern(retired).test(prose(carried ?? "")),
    );
  });
  const gated = gatedNeverWords(neverColumn);
  return [
    ...copies
      .filter(({ catalog }) => Object.keys(catalog).length === 0)
      .map(({ source }) => `reads a catalog with no entries: ${source}`),
    ...offenders("uses no en dash, em dash or double hyphen", /[–—]|--/),
    ...offenders("uses no straight double quote", /"/),
    ...offenders(
      "uses no straight apostrophe between letters",
      new RegExp(`[${LETTER}]'[${LETTER}]`),
    ),
    ...offenders("quotes with „…“ only", /[„“”»«]/, { text: unpaired }),
    ...offenders("spells an ellipsis as the one character …", /\.\.\./),
    // Edge whitespace is not checked: some values are fragments the code
    // joins around markup, and the joined text needs their edge space.
    ...offenders("has no double space", / {2}/),
    ...offenders("uses no exclamation mark", /!/),
    ...offenders(
      "says bitte once, to open a recovery step in a sentence",
      misplacedBitte(RECOVERY_BITTE_DU),
      { exempt: readByAnOutsider },
    ),
    ...offenders(
      "says bitte once, to open a recovery step in a sentence",
      misplacedBitte(RECOVERY_BITTE_SIE),
      { exempt: (key) => !readByAnOutsider(key) },
    ),
    // Case-sensitive past the first letter, so the certificate authority "CA."
    // closing a sentence is not read as "ca.".
    ...offenders("uses no abbreviation", ABBREVIATION, { text: prose }),
    ...offenders(
      "uses no apostrophe contraction",
      new RegExp(`[${LETTER}]['’]s(?![${LETTER}])`),
      { text: prose },
    ),
    // Read on the raw value: `{pct}%` is a number the formatter supplies,
    // glued to its sign as a literal digit would be.
    ...offenders("sets a no-break space before a percent sign", /[\d}] ?%/),
    ...offenders(
      "writes no gender asterisk, colon, gap, slash or Binnen-I",
      /[*:_/-]innen|[a-zäöüß]Innen/,
      { text: prose },
    ),
    // "mir" and "Meine" stay legal: "Mir zuweisen" and "Meine Deals" name the
    // reader in a control, not a voice.
    ...offenders(
      "says ich only inside the onboarding conversation",
      word("ich|Ich|mich|Mich"),
      {
        text: prose,
        exempt: (key) =>
          key.startsWith(AGENT_SPEAKS_PREFIX) || spokenByTheReader(key),
      },
    ),
    ...offenders(
      "never speaks as wir",
      word(`wir|Wir|uns|Uns|unser[${LETTER}]*|Unser[${LETTER}]*`),
      {
        text: prose,
        exempt: (key) =>
          key.startsWith(CONTROLLER_SPEAKS_PREFIX) || spokenByTheReader(key),
      },
    ),
    ...RETIRED_WORDS.flatMap((retired) =>
      offenders("uses no retired word", retiredWordPattern(retired), {
        text: prose,
        exempt: (key) => RETIRED_WORD_KEPT.has(key),
      }),
    ),
    ...stale.map(
      (key) => `keeps a retired word only where a catalog carries it: ${key}`,
    ),
    ...(gated.length > 0
      ? []
      : ["the Vocabulary table's Never column sets no word in code"]),
    ...RETIRED_WORDS.filter((retired) => !gated.includes(retired)).map(
      (retired) => `retires a word the Vocabulary table does not: ${retired}`,
    ),
    ...gated
      .filter((one) => !RETIRED_WORDS.includes(one))
      .map((one) => `misses a word the Vocabulary table retires: ${one}`),
  ];
}

export const rule: CensusRule = {
  name: "copy-style-de",
  corpus: (tree) => tree.files.filter((file) => file.path === coreCatalog),
  floors: () =>
    listedUnitCopy("de").length > 0 ? [] : ["no unit ships a de.json"],
  across: (corpus) => {
    const core = corpus[0]?.source;
    return core ? styleFindings(germanCopies(core)) : [];
  },
  advice:
    "German copy breaks docs/reference/ui-copy-style-de.md. Each line names " +
    "the check, the catalog, the key and the value to rewrite.",
};
