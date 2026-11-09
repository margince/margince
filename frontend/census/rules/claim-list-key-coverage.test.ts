// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  declaredListKeys,
  keysModule,
  rule,
} from "./claim-list-key-coverage.census";

const KEYS = `export const RECORD_LIST_KEY: Record<ListedRecordType, string> = {
  company: "companies",
  contact: "contact",
};
`;

describe("the claim-list-key-coverage census", () => {
  it("reads the keys off the table", () => {
    expect(declaredListKeys(KEYS)).toEqual(["companies", "contact"]);
    expect(declaredListKeys("export const OTHER = {};")).toBeNull();
  });
});

describe("the claim-list-key-coverage rule", () => {
  it("reports a key no query spells", () => {
    expect(
      plantedFindings(rule, [
        { path: keysModule, area: "src", text: KEYS },
        {
          path: join(srcDir, "features", "planted.tsx"),
          area: "src",
          text: 'useQuery({ queryKey: ["companies"] });\nuseQuery({ queryKey: ["contacts"] });\n',
        },
      ]),
    ).toEqual(['"contact" is invalidated by a claim and queried nowhere']);
  });
});
