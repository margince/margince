// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { plantedFindings } from "../tree";
import {
  coreCatalog,
  rule,
  styleFindings,
  vocabularyNeverColumn,
} from "./copy-style-de.census";

const findings = (catalog: Record<string, string>) =>
  styleFindings(
    [
      {
        source: "de.ts",
        catalog: { "a.ok": "Gespeichert.", ...catalog },
        english: {},
      },
    ],
    vocabularyNeverColumn(),
  ).filter((line) => !line.startsWith("keeps a retired word"));

describe("the de copy checks, over planted catalogs", () => {
  it("pass clean copy", () => {
    expect(findings({ "a.b": "Änderungen speichern" })).toEqual([]);
  });

  it.each([
    ["uses no en dash, em dash or double hyphen", "A – B"],
    ["uses no straight double quote", 'Öffne "Datei"'],
    ["uses no straight apostrophe between letters", "geht's"],
    ["quotes with „…“ only", "Öffne „Datei"],
    ["spells an ellipsis as the one character …", "Lädt..."],
    ["has no double space", "Speichern  jetzt"],
    ["uses no exclamation mark", "Fertig!"],
    ["says bitte once, to open a recovery step in a sentence", "Bitte warten"],
    ["uses no abbreviation", "Kontakte usw. laden."],
    ["uses no apostrophe contraction", "Lars’s Deals"],
    ["sets a no-break space before a percent sign", "{pct} %"],
    ["writes no gender asterisk, colon, gap, slash or Binnen-I", "Kund*innen"],
    ["says ich only inside the onboarding conversation", "Ich lade"],
    ["never speaks as wir", "Wir laden"],
    ["uses no retired word", "Die Firma"],
  ])("%s", (check, value) => {
    expect(
      findings({ "a.b": value }).filter((line) =>
        line.startsWith(`${check}: `),
      ),
    ).toEqual([`${check}: de.ts a.b: ${JSON.stringify(value)}`]);
  });

  it("holds the retired words to the Vocabulary table, both ways", () => {
    const column = " `Firma` and `Kiste` ";
    const lines = styleFindings([], column);
    expect(lines).toContain(
      "misses a word the Vocabulary table retires: Kiste",
    );
    expect(lines).toContain(
      "retires a word the Vocabulary table does not: Funnel",
    );
    expect(styleFindings([], "")).toContain(
      "the Vocabulary table's Never column sets no word in code",
    );
  });

  it("names an empty catalog", () => {
    expect(
      styleFindings([{ source: "x.json", catalog: {}, english: {} }]),
    ).toContain("reads a catalog with no entries: x.json");
  });
});

describe("the copy-style-de rule", () => {
  it("reports a planted value through its own corpus and across", () => {
    const text = 'export const de = { "a.b": "Fertig!" };\n';
    expect(
      plantedFindings(rule, [{ path: coreCatalog, area: "src", text }]),
    ).toContain(
      'uses no exclamation mark: frontend/src/i18n/de.ts a.b: "Fertig!"',
    );
  });
});
