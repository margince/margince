// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { plantedFindings } from "../tree";
import {
  copyFindings,
  coreCatalog,
  englishGate,
  germanCatalogs,
  rule,
  type Voices,
} from "./copy-style-vi.census";

const voices: Voices = {
  agentSpeaks: "ob.conv.",
  controllerSpeaks: "privacynotice.",
  spokenByTheReader: new Set(["prefs.wording.mine"]),
  spokenByTheReaderPrefix: "prefs.wording.",
  readByAnOutsider: ["book."],
  serverWorded: new Set(["confirm.server"]),
};

// The Vocabulary table's Never column, as the style page writes one row.
const cells = [" `bản ghi` for a record (bản ghi DNS keeps it) "];

const findings = (catalog: Record<string, string>, english = {}) =>
  copyFindings(
    [
      {
        source: "vi.ts",
        catalog: { "book.title": "Đặt lịch", ...catalog },
        english,
      },
    ],
    voices,
    cells,
  );

const check = (found: readonly string[], name: string) =>
  found.filter((line) => line.startsWith(`${name}: `));

describe("the vi copy checks, over planted catalogs", () => {
  it("pass clean copy", () => {
    expect(findings({ "a.b": "Lưu thay đổi" })).toEqual([]);
  });

  it.each([
    ["is NFC", "Lưu thay đổi".normalize("NFD")],
    ["marks the tone of an open oa, oe and uy on the first vowel", "Hoà"],
    ["marks the tone on the second vowel before a final consonant", "tóan"],
    ["uses no en dash, em dash, double hyphen or spaced hyphen", "A – B"],
    ["spells an ellipsis as the one character …", "Đang tải..."],
    ["uses no exclamation mark", "Xong!"],
    ["quotes with “…” and ’ only", 'Mở "tệp"'],
    ["has no double space", "Lưu  lại"],
    ["uses no abbreviation or ampersand", "A & B"],
    ["never speaks as chúng ta or mình", "Chúng ta lưu"],
    ["never addresses anyone as quý vị, anh/chị or kính thưa", "Kính thưa"],
    [
      "says quý khách and vui lòng only where an outsider reads",
      "Vui lòng lưu",
    ],
    ["capitalises bạn and quý khách only where a sentence opens", "Lưu Bạn"],
    ["uses no retired word", "Mở bản ghi"],
    ["says thất bại only in a sentence, never in a short value", "Thất bại"],
  ])("refuses copy that breaks %s", (name, value) => {
    expect(check(findings({ "a.b": value }), name)).toHaveLength(1);
  });

  it("keeps the ordinary compound of a retired word", () => {
    expect(findings({ "a.b": "Sửa bản ghi DNS" })).toEqual([]);
  });

  it("leaves server-worded keys to the server copy", () => {
    expect(findings({ "confirm.server": "Chúng ta đồng ý" })).toEqual([]);
  });

  it("says tôi only where an English me or a speaker allows it", () => {
    expect(
      check(
        findings({ "a.b": "Giao cho tôi" }, { "a.b": "Assign" }),
        "says tôi only for the onboarding agent, a consent statement or an English me",
      ),
    ).toHaveLength(1);
    expect(
      findings({ "a.b": "Giao cho tôi" }, { "a.b": "Assign to me" }),
    ).toEqual([]);
    expect(findings({ "ob.conv.hi": "Tôi giúp" })).toEqual([]);
  });

  it("refuses bạn where an outsider reads", () => {
    expect(
      check(
        findings({ "book.hi": "Bạn đặt lịch" }),
        "never says bạn where an outsider reads",
      ),
    ).toHaveLength(1);
  });

  it("writes the _one arm as the _other arm", () => {
    expect(
      check(
        findings(
          { "a.n_one": "{n} mục", "a.n_other": "{n} các mục" },
          { "a.n_one": "{n} item", "a.n_other": "{n} items" },
        ),
        "writes the _one arm as the _other arm",
      ),
    ).toHaveLength(1);
  });

  it("names an outsider family no catalog key carries", () => {
    expect(
      copyFindings(
        [{ source: "vi.ts", catalog: { "a.b": "Lưu" }, english: {} }],
        voices,
        cells,
      ),
    ).toEqual(["every outsider family names a key the catalogs carry: book."]);
  });

  it("refuses a unit value that is its English", () => {
    const found = copyFindings(
      [
        { source: "vi.ts", catalog: { "book.a": "Lưu" }, english: {} },
        {
          source: "unit/vi.json",
          catalog: { "u.a": "Save draft" },
          english: { "u.a": "Save draft" },
        },
      ],
      voices,
      cells,
    );
    expect(check(found, "ships no unit value as its English")).toHaveLength(1);
  });
});

describe("the copy-style-vi rule", () => {
  // The two modules the voices are read from are planted with their real text.
  const real = (path: string) => ({
    path,
    area: path.includes("/scripts/") ? ("scripts" as const) : ("src" as const),
    text: readFileSync(path, "utf8"),
  });

  it("reads the core catalog from the tree and refuses a planted value", () => {
    const found = plantedFindings(rule, [
      {
        path: coreCatalog,
        area: "src",
        text: 'export const vi = { "book.title": "Đặt lịch!" };',
      },
      real(englishGate),
      real(germanCatalogs),
    ]);
    expect(check(found, "uses no exclamation mark")).toEqual([
      'uses no exclamation mark: frontend/src/i18n/vi.ts book.title: "Đặt lịch!"',
    ]);
  });
});
