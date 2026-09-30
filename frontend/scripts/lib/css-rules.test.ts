// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  alternativesOf,
  classesOf,
  compounds,
  resolveNesting,
  rules,
  selectorList,
  splitTopLevel,
  subjectClasses,
  subjectOf,
  subjectsOf,
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

  it("counts from an :is() only what the element must carry either way", () => {
    expect([...classesOf(":is(.btn, .menu > .btn).x")]).toEqual(["x", "btn"]);
    expect([...classesOf(".y:matches(.a.b, .b)")]).toEqual(["y", "b"]);
    expect([...classesOf(":-webkit-any(.a)")]).toEqual(["a"]);
  });

  it("reads no class out of an attribute value, and all of an escaped one", () => {
    expect([...classesOf('.a[href$=".pdf"]')]).toEqual(["a"]);
    expect([...classesOf(".sm\\:flex.w-1\\/2")]).toEqual(["sm:flex", "w-1/2"]);
  });
});

describe("the alternatives of a selector", () => {
  it("reads each :is() or :where() alternative as a selector of its own", () => {
    expect(alternativesOf(":is(.token, .pill) .x")).toEqual([
      ".token .x",
      ".pill .x",
    ]);
    expect(alternativesOf(".bar > :where(.btn, .chip):hover")).toEqual([
      ".bar > .btn:hover",
      ".bar > .chip:hover",
    ]);
  });

  it("distributes every alternation, nested ones included", () => {
    expect(alternativesOf(":is(.a, :is(.b, .c)) + :where(.x, .y)")).toEqual([
      ".a + .x",
      ".a + .y",
      ".b + .x",
      ".b + .y",
      ".c + .x",
      ".c + .y",
    ]);
  });

  it("hangs a complex alternative's ancestors in front of the compound", () => {
    expect(alternativesOf("button:is(.menu > .b).c")).toEqual([
      ".menu > button.b.c",
    ]);
    expect(alternativesOf(".x:is(span, .y)")).toEqual(["span.x", ".x.y"]);
  });

  it("drops an alternative whose element type contradicts the compound's", () => {
    expect(alternativesOf("div:is(a, .b)")).toEqual(["div.b"]);
    expect(alternativesOf("DIV:is(div, .b)")).toEqual(["DIV", "DIV.b"]);
    expect(alternativesOf("*:is(a, .b)")).toEqual(["a", "*.b"]);
  });

  it("leaves an alternation that names another element alone", () => {
    expect(alternativesOf(".a:not(:is(.b, .c)) .d")).toEqual([
      ".a:not(:is(.b, .c)) .d",
    ]);
  });

  it("gives every alternative a subject", () => {
    expect(subjectsOf(":is(.btn, .iconbtn)")).toEqual([".btn", ".iconbtn"]);
  });
});

describe("text the tokenizer cannot balance", () => {
  it("throws rather than reading the rest as one part", () => {
    for (const text of [
      ".a), .b, .c",
      ".a(, .b",
      '.a[x="open, .b',
      ".a[x, .b",
    ]) {
      expect(() => selectorList(text), text).toThrow(/unbalanced/);
    }
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

  it("refuses a sheet that nests one rule inside another", () => {
    writeFileSync(
      join(root, "sheet.css"),
      ".a { color: red; .b { color: blue; } }",
    );
    expect(() => rules(root)).toThrow(/sheet\.css nests a rule/);
  });

  it("reads one rule per selector, not per comma", () => {
    writeFileSync(
      join(root, "sheet.css"),
      ":is(.a, .b) .c,\n.d { color: var(--textPrimary); }",
    );
    expect(rules(root).map(({ selector }) => selector)).toEqual([
      ":is(.a, .b) .c",
      ".d",
    ]);
  });
});
