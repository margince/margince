// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The mechanical half of docs/reference/ui-copy-style-vi.md. Each check lists
// every offender, so one run names all the strings a rewrite has to touch.
//
// The core catalog is read from the tree's vi.ts. A unit ships its copy as
// vi.json beside en.json, found in the git listing.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import {
  catalogDeclaredIn,
  catalogEntries,
  type LocaleCopy,
  type LocaleEntry,
  listedUnitCopy,
} from "../../scripts/lib/locale-catalogs";
import { en } from "../../src/i18n/en";
import { byCodeUnit } from "../../src/lib/codeunitorder";
import {
  type CensusRule,
  type CensusTree,
  repoRoot,
  scriptsDir,
  srcDir,
} from "../tree";

export const coreCatalog = join(srcDir, "i18n", "vi.ts");
export const englishGate = join(srcDir, "i18n", "copy-style.test.ts");
export const germanCatalogs = join(scriptsDir, "lib", "german-catalogs.ts");

type Catalog = Readonly<Record<string, string>>;

function parsedAt(tree: CensusTree, path: string): ts.SourceFile {
  const source = tree.at(path)?.source;
  if (!source) {
    throw new Error(`${path} is not in the census tree; this rule reads it`);
  }
  return source;
}

function literalsOf(initializer: ts.Expression): string[] {
  if (ts.isObjectLiteralExpression(initializer)) {
    return initializer.properties.flatMap((property) =>
      property.name && ts.isStringLiteral(property.name)
        ? [property.name.text]
        : [],
    );
  }
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isStringLiteral(node)) found.push(node.text);
    ts.forEachChild(node, visit);
  };
  visit(initializer);
  return found;
}

/** The strings a module declares under `name`: an object's keys, or literals. */
function declaredStrings(source: ts.SourceFile, name: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer
    ) {
      found.push(...literalsOf(node.initializer));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  if (found.length === 0) {
    throw new Error(
      `${source.fileName} no longer declares ${name}; this rule reads it`,
    );
  }
  return found;
}

function declaredString(source: ts.SourceFile, name: string): string {
  const [only, ...rest] = declaredStrings(source, name);
  if (only === undefined || rest.length > 0) {
    throw new Error(`${source.fileName}: ${name} no longer holds one string`);
  }
  return only;
}

// The server publishes these consent answers and a proof records their
// wording, so the wording checks leave them to the server copy.
function serverWordedKeys(): Set<string> {
  const gate = readFileSync(
    join(repoRoot, "backend", "gates", "marketingquestion_test.go"),
    "utf8",
  );
  const keys = [...gate.matchAll(/\{"([\w.]+)", "Confirm\w+"\}/g)].map(
    ([, key]) => key,
  );
  if (keys.length === 0) {
    throw new Error("marketingquestion_test.go no longer pairs screen keys");
  }
  return new Set(keys);
}

// A placeholder names a parameter, not copy. A space keeps the words on either
// side from reading as one.
export function prose(value: string): string {
  return value.replace(/\{[^}]*\}/g, " ");
}

// A vendor's label quoted in “…” stays as the English catalog quotes it.
function unquoted(value: string): string {
  return prose(value).replace(/“[^“”]*”/g, " ");
}

export function word(alternation: string, flags = "iu"): RegExp {
  return new RegExp(`(?<![\\p{L}])(?:${alternation})(?![\\p{L}])`, flags);
}

function escaped(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

const WEB_ADDRESS = /[a-z][a-z\d+.-]*:\/\/[^\s“”()]+/gu;
const FILE_NAME = /[^\s“”(),]+\.[a-z][a-z\d]{0,4}(?![\p{L}\d])/gu;

// A file name or a URL keeps the spelling the reader must type.
function withoutPaths(value: string): string {
  return unquoted(value).replace(WEB_ADDRESS, " ").replace(FILE_NAME, " ");
}

function vocabularyNeverCells(): string[] {
  const page = readFileSync(
    join(repoRoot, "docs", "reference", "ui-copy-style-vi.md"),
    "utf8",
  );
  const section = page.split("\n## Vocabulary\n")[1]?.split("\n## ")[0] ?? "";
  return section
    .split("\n")
    .filter((line) => line.startsWith("|") && !line.startsWith("|---"))
    .slice(1)
    .map((row) => row.split("|")[3] ?? "");
}

function retiredWords(cells: readonly string[]): RegExp {
  const gated = cells.flatMap((cell) =>
    [...cell.matchAll(/`([^`]+)`/g)].map(([, retired]) => retired),
  );
  if (gated.length === 0) {
    throw new Error("the Vocabulary table's Never column sets no word in code");
  }
  return word(
    gated
      .map(
        (retired) =>
          `${escaped(retired)}${/^[\x20-\x7e]+$/.test(retired) ? "s?" : ""}`,
      )
      .join("|"),
  );
}

// A row's "(hiệu suất, xác suất and tần suất keep theirs)" names the compounds
// where a retired word keeps an ordinary sense.
function keptCompounds(cells: readonly string[], retired: RegExp): RegExp {
  const kept = cells
    .flatMap((cell) =>
      [...cell.matchAll(/\(([^()]*) (?:keep theirs|keeps it)\)/g)].flatMap(
        ([, list]) => list.split(/, | and /),
      ),
    )
    .filter((compound) => retired.test(compound));
  if (kept.length === 0) {
    throw new Error("the Vocabulary table keeps no compound of a retired word");
  }
  return word(kept.map(escaped).join("|"), "giu");
}

// "của mình", "riêng mình" and "chỉ mình" can mean the speaker's own, which
// only a consent statement may say.
const REFLEXIVE = word("(?:chính|tự|một) mình", "giu");

// What can stand before a capital that opens a sentence. `\}\s` because a
// placeholder's value may itself end a sentence.
const SENTENCE_OPENS = /(?:[.?:]\s|\n|[“(]|\}\s)$/;

function placeholderSet(value: string): string {
  return (value.match(/\{\w+\}/g) ?? []).sort(byCodeUnit).join();
}

function lettersOf(value: string): string {
  return (
    prose(value)
      .replace(WEB_ADDRESS, " ")
      .match(/\p{L}+/gu) ?? []
  ).join(" ");
}

/** What the checks read besides the catalogs: who speaks and who reads. */
export type Voices = Readonly<{
  agentSpeaks: string;
  controllerSpeaks: string;
  spokenByTheReader: ReadonlySet<string>;
  spokenByTheReaderPrefix: string;
  readByAnOutsider: readonly string[];
  serverWorded: ReadonlySet<string>;
}>;

function voicesIn(tree: CensusTree): Voices {
  const gate = parsedAt(tree, englishGate);
  return {
    agentSpeaks: declaredString(gate, "AGENT_SPEAKS_PREFIX"),
    controllerSpeaks: declaredString(gate, "CONTROLLER_SPEAKS_PREFIX"),
    spokenByTheReader: new Set(declaredStrings(gate, "SPOKEN_BY_THE_READER")),
    spokenByTheReaderPrefix: declaredString(
      gate,
      "SPOKEN_BY_THE_READER_PREFIX",
    ),
    // German's Sie families, plus the public booking form a guest fills in,
    // which Vietnamese alone addresses as an outsider.
    readByAnOutsider: [
      ...declaredStrings(parsedAt(tree, germanCatalogs), "READ_BY_AN_OUTSIDER"),
      "book.",
    ],
    serverWorded: serverWordedKeys(),
  };
}

/** Every finding over the given catalogs, each led by the check it breaks. */
export function copyFindings(
  copies: readonly LocaleCopy[],
  voices: Voices,
  cells: readonly string[] = vocabularyNeverCells(),
): string[] {
  const entries = catalogEntries(copies);
  const retired = retiredWords(cells);
  const ordinary = keptCompounds(cells, retired);
  const spokenByTheReader = (key: string) =>
    voices.spokenByTheReader.has(key) ||
    key.startsWith(voices.spokenByTheReaderPrefix);
  const readByAnOutsider = (key: string) =>
    voices.readByAnOutsider.some((prefix) => key.startsWith(prefix));
  const catalogOf = (source: string): Catalog =>
    copies.find((copy) => copy.source === source)?.catalog ?? {};
  const englishOf = (source: string): Catalog =>
    copies.find((copy) => copy.source === source)?.english ?? {};

  const offenders = (
    check: string,
    breaks: (entry: LocaleEntry) => boolean,
    corpus: readonly LocaleEntry[] = entries,
  ): string[] =>
    corpus
      .filter(breaks)
      .map(
        ([source, key, value]) =>
          `${check}: ${source} ${key}: ${JSON.stringify(value)}`,
      );

  const matching = (
    check: string,
    pattern: RegExp,
    options: {
      text?: (value: string) => string;
      exempt?: (entry: LocaleEntry) => boolean;
      wording?: boolean;
    } = {},
  ): string[] => {
    const {
      text = (value: string) => value,
      exempt = () => false,
      wording = false,
    } = options;
    return offenders(
      check,
      (entry) =>
        !(wording && voices.serverWorded.has(entry[1])) &&
        !exempt(entry) &&
        pattern.test(text(entry[2])),
    );
  };

  const wordsIn = (value: string) =>
    prose(value)
      .split(/\s+/)
      .filter((token) => /\p{L}/u.test(token)).length;

  return [
    ...offenders("is NFC", ([, , value]) => value !== value.normalize("NFC")),
    ...matching(
      "marks the tone of an open oa, oe and uy on the first vowel",
      /(?<!q)o[àáảãạ](?![\p{L}])|o[èéẻẽẹ](?![\p{L}])|(?<!q)u[ỳýỷỹỵ](?![\p{L}])/iu,
      { text: prose },
    ),
    ...matching(
      "marks the tone on the second vowel before a final consonant",
      /[òóỏõọ][ae]\p{L}|(?<!q)[ùúủũụ]y\p{L}/iu,
      { text: prose },
    ),
    ...matching(
      "uses no en dash, em dash, double hyphen or spaced hyphen",
      /[–—]|--|\s-\s/,
      { text: (value) => ` ${prose(value)} ` },
    ),
    ...matching("spells an ellipsis as the one character …", /\.\.\./),
    ...matching("uses no exclamation mark", /!/),
    ...matching("quotes with “…” and ’ only", /["„»«]|\p{L}'\p{L}/u),
    // Edge whitespace is not checked: some values are fragments the code joins
    // around markup, and they need their leading or trailing space.
    ...matching("has no double space", / {2}/),
    ...matching(
      "uses no abbreviation or ampersand",
      new RegExp(`${word("v\\.v\\.|vd\\.|v/v").source}|&`, "iu"),
      { text: unquoted },
    ),
    // Where the English arms carry different placeholders, i18n.test.ts makes
    // the Vietnamese arms differ too, and the style page owns that shape.
    ...offenders(
      "writes the _one arm as the _other arm",
      ([source, key, value]) => {
        if (!key.endsWith("_one")) return false;
        const otherKey = key.replace(/_one$/, "_other");
        const other = catalogOf(source)[otherKey];
        const english = englishOf(source);
        return (
          other !== undefined &&
          placeholderSet(english[key] ?? "") ===
            placeholderSet(english[otherKey] ?? "") &&
          value !== other
        );
      },
    ),
    // "Giao cho tôi" and "Deal của tôi" name the reader in a control, as the
    // English "Assign to me" and "My deals" do.
    ...matching(
      "says tôi only for the onboarding agent, a consent statement or an English me",
      word("tôi"),
      {
        text: (value) => prose(value).replace(word("chúng tôi", "giu"), " "),
        exempt: ([, key, , english]) =>
          key.startsWith(voices.agentSpeaks) ||
          spokenByTheReader(key) ||
          /\b(?:me|my|mine)\b/i.test(english),
        wording: true,
      },
    ),
    ...matching(
      "says chúng tôi only as the data controller or in a consent statement",
      word("chúng tôi"),
      {
        text: prose,
        exempt: ([, key]) =>
          key.startsWith(voices.controllerSpeaks) || spokenByTheReader(key),
        wording: true,
      },
    ),
    ...matching("never speaks as chúng ta or mình", word("chúng ta|mình"), {
      text: (value) => prose(value).replace(REFLEXIVE, " "),
      exempt: ([, key]) => spokenByTheReader(key),
      wording: true,
    }),
    ...matching(
      "never addresses anyone as quý vị, anh/chị or kính thưa",
      word("quý vị|anh/chị|anh chị(?! em)|kính thưa"),
      { text: prose, wording: true },
    ),
    ...matching(
      "says quý khách and vui lòng only where an outsider reads",
      word("quý khách|vui lòng"),
      {
        text: prose,
        exempt: ([, key]) => readByAnOutsider(key),
        wording: true,
      },
    ),
    ...matching("never says bạn where an outsider reads", word("bạn"), {
      text: prose,
      exempt: ([, key]) => !readByAnOutsider(key),
      wording: true,
    }),
    ...offenders(
      "capitalises bạn and quý khách only where a sentence opens",
      ([, , value]) =>
        [...value.matchAll(/(?<![\p{L}])(?:Bạn|Quý khách)(?![\p{L}])/gu)].some(
          (hit) =>
            hit.index > 0 && !SENTENCE_OPENS.test(value.slice(0, hit.index)),
        ),
    ),
    ...voices.readByAnOutsider
      .filter((prefix) => !entries.some(([, key]) => key.startsWith(prefix)))
      .map(
        (prefix) =>
          `every outsider family names a key the catalogs carry: ${prefix}`,
      ),
    ...matching("uses no retired word", retired, {
      text: (value) => withoutPaths(value).replace(ordinary, " "),
      wording: true,
    }),
    // A short value is a title, label, status or counter, where a failure is
    // "bị lỗi" or "không thành công".
    ...offenders(
      "says thất bại only in a sentence, never in a short value",
      ([, , value]) => word("thất bại").test(value) && wordsIn(value) <= 8,
    ),
    ...copies
      .filter(({ catalog }) => Object.keys(catalog).length === 0)
      .map(({ source }) => `reads a catalog with no entries: ${source}`),
    ...offenders(
      "ships no unit value as its English",
      ([, , value, english]) =>
        lettersOf(value) !== "" && lettersOf(value) === lettersOf(english),
      entries.filter(([source]) => source !== copies[0]?.source),
    ),
  ];
}

export const rule: CensusRule = {
  name: "copy-style-vi",
  corpus: (tree) => tree.files.filter((file) => file.path === coreCatalog),
  across: (corpus, tree) => {
    const core = corpus[0]?.source;
    if (!core) return [];
    const copies: LocaleCopy[] = [
      {
        source: "frontend/src/i18n/vi.ts",
        catalog: catalogDeclaredIn(core, "vi"),
        english: en,
      },
      ...listedUnitCopy("vi"),
    ];
    return copyFindings(copies, voicesIn(tree));
  },
  advice:
    "Vietnamese copy breaks docs/reference/ui-copy-style-vi.md. Each line " +
    "names the check, the catalog, the key and the value to rewrite.",
};
