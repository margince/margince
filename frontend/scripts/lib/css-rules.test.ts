// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  classesOf,
  compounds,
  resolveNesting,
  rules,
  selectorList,
  splitTopLevel,
  subjectClasses,
  subjectOf,
} from "./css-rules";

describe("a selector list", () => {
  it("splits at the commas between selectors", () => {
    expect(selectorList(".a,\n.b .c, .d")).toEqual([".a", ".b .c", ".d"]);
  });

  it("keeps a comma inside a functional pseudo-class", () => {
    expect(selectorList(":is(.a, .b) .c")).toEqual([":is(.a, .b) .c"]);
    expect(selectorList(".e:not(:is(.a, .b)), .f")).toEqual([
      ".e:not(:is(.a, .b))",
      ".f",
    ]);
  });

  it("keeps a comma inside an attribute value or behind an escape", () => {
    expect(selectorList('[data-x="a, b"], .c')).toEqual([
      '[data-x="a, b"]',
      ".c",
    ]);
    expect(selectorList(".a\\,b, .c")).toEqual([".a\\,b", ".c"]);
  });
});

describe("the compounds of a selector", () => {
  it("splits at every combinator", () => {
    expect(compounds(".a > .b ~ .c + .d .e")).toEqual([
      ".a",
      ".b",
      ".c",
      ".d",
      ".e",
    ]);
  });

  it("keeps a combinator inside a pseudo-class argument", () => {
    expect(compounds(".a:has(> .x)")).toEqual([".a:has(> .x)"]);
    expect(compounds("li:nth-of-type(3n + 1) .b")).toEqual([
      "li:nth-of-type(3n + 1)",
      ".b",
    ]);
  });

  it("keeps a combinator inside a quoted attribute value", () => {
    expect(compounds("[title='x > y'] .z")).toEqual(["[title='x > y']", ".z"]);
  });
});

describe("the subject of a selector", () => {
  it("is the last compound, not one inside a pseudo-class", () => {
    expect(subjectOf(":is(.a, .b) .c")).toBe(".c");
    expect(subjectOf(".a:has(> .x)")).toBe(".a:has(> .x)");
  });

  it("carries only its own classes", () => {
    expect([...subjectClasses(".a:has(> .x)")]).toEqual(["a"]);
    expect([...subjectClasses(".d:not(:has(> .x))")]).toEqual(["d"]);
    expect([...subjectClasses(".m .n.o")]).toEqual(["n", "o"]);
  });
});

describe("the classes of a compound", () => {
  it("leaves out every class named inside a nested pseudo-class", () => {
    expect([...classesOf(".e:not(:is(.a, .b))")]).toEqual(["e"]);
    expect([...classesOf(".e:not(:has(.x)).f")]).toEqual(["e", "f"]);
  });

  it("carries what every alternative of an :is() or :where() carries", () => {
    expect([...classesOf(":is(.btn, .menu > .btn).x")]).toEqual(["x", "btn"]);
    expect([...classesOf(":where(h1, .title)")]).toEqual([]);
    expect([...classesOf(".y:is(.a.b, .b)")]).toEqual(["y", "b"]);
  });
});

describe("a nested selector list", () => {
  it("stands alone at the top level", () => {
    expect(resolveNesting([], ".a, :is(.b, .c)")).toEqual([
      ".a",
      ":is(.b, .c)",
    ]);
  });

  it("puts `&` in the outer selector's place and descends without one", () => {
    expect(resolveNesting([".card", ".panel"], "&:hover, .title")).toEqual([
      ".card:hover",
      ".panel:hover",
      ".card .title",
      ".panel .title",
    ]);
  });
});

describe("a top-level split of a value", () => {
  it("keeps a function's own arguments together", () => {
    expect(splitTopLevel("var(--a, 10px) 9px", " ")).toEqual([
      "var(--a, 10px)",
      "9px",
    ]);
  });
});

describe("the rules under a tree", () => {
  let root = "";
  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "css-rules-"));
  });
  afterEach(() => {
    rmSync(root, { recursive: true, force: true });
  });

  it("reads one rule per selector, not per comma", () => {
    writeFileSync(
      join(root, "sheet.css"),
      ":is(.a, .b) .c,\n.d { color: var(--ink); }",
    );
    expect(rules(root).map(({ selector }) => selector)).toEqual([
      ":is(.a, .b) .c",
      ".d",
    ]);
  });
});
