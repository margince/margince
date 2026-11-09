// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { READ_BY_AN_OUTSIDER } from "../../scripts/lib/german-catalogs";
import { plantedFindings } from "../tree";
import { registerFindings, rule } from "./address-register.census";
import { coreCatalog } from "./copy-style-de.census";

// Every exempt family carries one key, so the prefix check stays quiet.
const exempt = Object.fromEntries(
  Object.keys(READ_BY_AN_OUTSIDER).map((prefix) => [`${prefix}x`, "Gut."]),
);

const findings = (catalog: Record<string, string>) =>
  registerFindings([
    { source: "de.ts", catalog: { ...exempt, ...catalog }, english: {} },
  ]);

describe("the address checks, over planted catalogs", () => {
  it("pass du copy, and Sie in a family an outsider reads", () => {
    const outsider = Object.keys(READ_BY_AN_OUTSIDER)[0];
    expect(
      findings({
        "a.b": "Speichere deine Änderungen. Du kannst sie später sehen.",
        "a.c": "{detail} Du kannst es erneut versuchen.",
        [`${outsider}y`]: "Bitte bestätigen Sie Ihre Adresse.",
      }),
    ).toEqual([]);
  });

  it("names a formal address, and where in the sentence it stands", () => {
    expect(findings({ "a.b": "Speichern Sie Ihre Daten." })).toEqual([
      'addresses the reader as Sie or Ihr: de.ts a.b: "Sie" mid-sentence in Speichern Sie Ihre Daten.',
      'addresses the reader as Sie or Ihr: de.ts a.b: "Ihre" mid-sentence in Speichern Sie Ihre Daten.',
    ]);
    expect(findings({ "a.b": "Ihre Daten." })).toEqual([
      'addresses the reader as Sie or Ihr: de.ts a.b: "Ihre" opening a sentence in Ihre Daten.',
    ]);
  });

  it("names a du-pronoun capitalised mid-sentence", () => {
    expect(findings({ "a.b": "Speichere Deine Daten." })).toEqual([
      'capitalises a du-pronoun mid-sentence: de.ts a.b: "Deine" in Speichere Deine Daten.',
    ]);
  });

  it("names an exempt prefix no key carries", () => {
    const [first] = Object.keys(READ_BY_AN_OUTSIDER);
    expect(
      registerFindings([{ source: "de.ts", catalog: {}, english: {} }]),
    ).toContain(`every exempt prefix names a key the catalogs carry: ${first}`);
  });
});

describe("the address-register rule", () => {
  it("reports a planted value through its own corpus and across", () => {
    const text = 'export const de = { "a.b": "Speichern Sie." };\n';
    expect(
      plantedFindings(rule, [{ path: coreCatalog, area: "src", text }]),
    ).toContain(
      'addresses the reader as Sie or Ihr: frontend/src/i18n/de.ts a.b: "Sie" mid-sentence in Speichern Sie.',
    );
  });
});
