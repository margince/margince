// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, repoRoot } from "../tree";
import {
  coreCatalog,
  neverColumnCountFindings,
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
    ["keeps the -e of a du imperative", "Schreib zuerst einen Grund."],
    ["keeps the -e of a du imperative", "Versuch es erneut."],
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

  // Freigeben is approval. A value whose English shares says teilen, and one
  // whose English also approves keeps the approval word.
  it("says teilen where the English says share", () => {
    const shared = styleFindings([
      {
        source: "de.ts",
        catalog: {
          "a.share": "Für dich freigegeben",
          "a.opens": "Freigeben für das Team",
          "a.compound": "Keine Kontofreigabe vorhanden",
          "a.split": "Gib die Kontakte für das Team frei.",
          "a.approve": "Wird erst nach einer Freigabe geteilt.",
          "a.free": "Geteilt, die Zeit bleibt frei.",
          "a.ok": "Mit dir geteilt",
        },
        english: {
          "a.share": "Shared with me",
          "a.opens": "Share with the team",
          "a.compound": "No account sharing",
          "a.split": "Share the contacts with the team.",
          "a.approve": "This share takes effect only after approval.",
          "a.free": "Shared, and the time stays free.",
          "a.ok": "Shared with me",
        },
      },
    ]).filter((line) => line.startsWith("says teilen"));
    expect(shared).toEqual([
      'says teilen where the English says share: de.ts a.share: "Für dich freigegeben"',
      'says teilen where the English says share: de.ts a.opens: "Freigeben für das Team"',
      'says teilen where the English says share: de.ts a.compound: "Keine Kontofreigabe vorhanden"',
      'says teilen where the English says share: de.ts a.split: "Gib die Kontakte für das Team frei."',
    ]);
  });

  it("retires exactly the words the Vocabulary table sets in code", () => {
    const lines = styleFindings(
      [
        {
          source: "de.ts",
          catalog: { "a.box": "Eine Kiste", "a.plain": "Eine Truhe" },
          english: {},
        },
      ],
      " `Kiste`, Truhe ",
    ).filter((line) => line.startsWith("uses no retired word"));
    expect(lines).toEqual(['uses no retired word: de.ts a.box: "Eine Kiste"']);
    expect(styleFindings([], "")).toContain(
      "the Vocabulary table's Never column sets no word in code",
    );
  });

  it("parses exactly the count of retired words the real page states", () => {
    expect(neverColumnCountFindings()).toEqual([]);
    const page = readFileSync(
      join(repoRoot, "docs/reference/ui-copy-style-de.md"),
      "utf8",
    );
    const lostRow = page.replace(/^\|[^\n]*`Firma`[^\n]*\n/m, "");
    expect(neverColumnCountFindings(lostRow)).toHaveLength(1);
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
