// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { READ_BY_AN_OUTSIDER } from "../../scripts/lib/german-catalogs";
import { plantedFindings as planted, srcDir, treeOf } from "../tree";
import {
  buyerPageFiles,
  buyerPageFindings,
  registerFindings,
  rule,
} from "./address-register.census";
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

describe("the buyer's page, over planted catalogs", () => {
  const copies = [
    {
      source: "de.ts",
      catalog: {
        "threads.readOnly": "Du hast nur Lesezugriff.",
        "threads.empty": "Noch keine Nachrichten.",
        "buyer.previewBanner": "Du kannst alles lesen.",
        "common.retry": "Versuche es, bevor du gehst.",
      },
      english: {},
    },
  ];
  const page = (text: string) => [
    { rel: "src/features/deals/buyerroom.tsx", text },
  ];

  it("names a du line the page shows from another family, in any case", () => {
    expect(
      buyerPageFindings(
        copies,
        page('t("threads.readOnly"); t("common.retry");'),
      ),
    ).toEqual([
      "says du on the buyer's page: src/features/deals/buyerroom.tsx names de.ts threads.readOnly: Du hast nur Lesezugriff.",
      "says du on the buyer's page: src/features/deals/buyerroom.tsx names de.ts common.retry: Versuche es, bevor du gehst.",
    ]);
  });

  it("passes neutral copy, and a buyer. key, and a key the page never names", () => {
    expect(
      buyerPageFindings(
        copies,
        page('t("threads.empty"); t("buyer.previewBanner");'),
      ),
    ).toEqual([]);
  });
});

describe("the buyer's page is its entry and the deal files it imports", () => {
  const deals = join(srcDir, "features", "deals");
  const file = (name: string, text: string) => ({
    path: join(deals, name),
    area: "src" as const,
    text,
  });

  it("follows an import into a shared deal file, and stops outside deals", () => {
    const tree = treeOf([
      file("buyerroom.tsx", 'import { Board } from "./dealroomthreads";'),
      file(
        "dealroomthreads.tsx",
        'import { x } from "../../lib/problem";\nt("threads.readOnly");',
      ),
      file("dealroom.tsx", 't("threads.readOnly");'),
      { path: join(srcDir, "lib", "problem.ts"), area: "src", text: "" },
    ]);
    expect(buyerPageFiles(tree).map((found) => found.rel)).toEqual([
      "src/features/deals/buyerroom.tsx",
      "src/features/deals/dealroomthreads.tsx",
    ]);
  });
});

describe("the address-register rule", () => {
  it("reports a planted value through its own corpus and across", () => {
    const text = 'export const de = { "a.b": "Speichern Sie." };\n';
    expect(planted(rule, [{ path: coreCatalog, area: "src", text }])).toContain(
      'addresses the reader as Sie or Ihr: frontend/src/i18n/de.ts a.b: "Sie" mid-sentence in Speichern Sie.',
    );
  });
});
