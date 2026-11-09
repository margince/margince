// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { rule, schemaPath, writeFindings } from "./if-match-coverage.census";

// A contract in the generated shape: one route whose PATCH takes If-Match and
// whose GET does not.
const SCHEMA = `export interface paths {
  "/deals/{id}": {
    get: operations["getDeal"];
    patch: operations["updateDeal"];
  };
}
export interface operations {
  getDeal: { parameters: { header?: never } };
  updateDeal: {
    parameters: { header?: { "If-Match"?: string } };
  };
}
`;

const screen = join(srcDir, "features", "planted.tsx");

const tree = (text: string) =>
  treeOf([
    { path: schemaPath, area: "src", text: SCHEMA },
    { path: screen, area: "src", text },
  ]);

const found = (text: string, listed: readonly string[] = []) => {
  const planted = tree(text);
  return writeFindings(rule.corpus(planted), planted, listed);
};

describe("the if-match-coverage census", () => {
  it("reports a write that sends no precondition", () => {
    expect(found(`api.PATCH("/deals/{id}", { body });`)).toEqual([
      "features/planted.tsx PATCH /deals/{id}",
    ]);
  });

  it("reads a precondition off the syntax tree, not the text", () => {
    const commented = `api.PATCH("/deals/{id}", { body, /* ifMatch(v) */ });`;
    expect(found(commented)).toEqual([
      "features/planted.tsx PATCH /deals/{id}",
    ]);
  });

  it("accepts a precondition spread into params, also behind a choice", () => {
    expect(
      found(`api.PATCH("/deals/{id}", { params: { ...ifMatch(v) }, body });`),
    ).toEqual([]);
    expect(
      found(
        `api.PATCH("/deals/{id}", { params: { ...(v ? ifMatch(v) : {}) } });`,
      ),
    ).toEqual([]);
  });

  it("leaves an endpoint that takes no precondition alone", () => {
    expect(found(`api.GET("/deals/{id}", {});`)).toEqual([]);
  });

  it("reports a listed write that now pins its row", () => {
    expect(found("", ["features/planted.tsx PATCH /deals/{id}"])).toEqual([
      "features/planted.tsx PATCH /deals/{id} now sends If-Match; delete its UNPINNED_WRITES line",
    ]);
  });
});

describe("the if-match-coverage rule", () => {
  it("reports a planted unpinned write through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      { path: schemaPath, area: "src", text: SCHEMA },
      { path: screen, area: "src", text: `api.PATCH("/deals/{id}", {});` },
    ]);
    expect(findings).toContain("features/planted.tsx PATCH /deals/{id}");
  });

  it("states a floor when the contract yields no conditional endpoint", () => {
    const planted = treeOf([{ path: screen, area: "src", text: "api.x" }]);
    expect(rule.floors?.(rule.corpus(planted), planted)).toEqual([
      "schema.d.ts yielded no endpoint that takes If-Match",
    ]);
  });
});
