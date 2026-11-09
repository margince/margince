// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { disclosuresIn, rule } from "./caught-error-coverage.census";

// Each case is a shape that shipped or is one edit from one. A census of zero
// cannot tell a clean tree from a blind detector, so each shape is planted.
describe("the caught-error-coverage census", () => {
  const cases: ReadonlyArray<readonly [string, string, number]> = [
    [
      "the ternary that shipped at four sites",
      "try { a(); } catch (err) { show(err instanceof Error ? err.message : String(err)); }",
      // Both halves of one ternary: `.message` and the `String` fallback.
      2,
    ],
    ["a bare .message", "try { a(); } catch (e) { show(e.message); }", 1],
    [
      "a cast in front of it",
      "try { a(); } catch (e) { show((e as Error).message); }",
      1,
    ],
    ["a non-null assertion", "try { a(); } catch (e) { show(e!.message); }", 1],
    [
      // `satisfies` narrows the type and keeps the value, so it is a wrapper
      // like the other four.
      "a satisfies expression",
      "try { a(); } catch (e) { show(((e as Error) satisfies Error).message); }",
      1,
    ],
    ["String() alone", "try { a(); } catch (e) { show(String(e)); }", 1],
    ["toString()", "try { a(); } catch (e) { show(e.toString()); }", 1],
    [
      "a template",
      // A template literal, so the planted case carries a real `${…}`.
      `try { a(); } catch (e) { show(\`failed: \${e}\`); }`,
      1,
    ],
    [
      "an alias, which a rename would otherwise hide",
      "try { a(); } catch (err) { const failure = err; show(failure.message); }",
      1,
    ],
    [
      "an alias through a cast",
      "try { a(); } catch (err) { const f = err as Error; show(String(f)); }",
      1,
    ],
    [
      "a mutation result's error, which carries no catch at all",
      "show(download.error.message);",
      1,
    ],
    [
      "a query result's error inside JSX",
      "const x = <p>{save.error.message}</p>;",
      1,
    ],
    [
      // The same read through the other accessor.
      "a bracketed read of the same property",
      'try { a(); } catch (e) { show(e["message"]); }',
      1,
    ],
    [
      // The bracket spelling on the first hop.
      "a result error reached by bracket",
      'show(download["error"].message);',
      1,
    ],
    ["both hops by bracket", 'show(save["error"]["message"]);', 1],
    ["only the message hop bracketed", 'show(download.error["message"]);', 1],
    [
      "a result's error behind a non-null assertion",
      "show(download.error!.message);",
      1,
    ],
    [
      "a result's error behind a cast",
      "show((save.error as Error).message);",
      1,
    ],
    [
      "a nested catch inside a handler",
      "try { a(); } catch (outer) { try { b(); } catch (inner) { show(inner.message); } }",
      1,
    ],
    // The correct spellings, which no arm reports.
    [
      "the correct spelling",
      "try { a(); } catch (err) { show(problemMessageOf(err, t)); }",
      0,
    ],
    [
      "the sanctioned console sink",
      "try { a(); } catch (err) { logUnexpectedError(err); }",
      0,
    ],
    ["a rethrow", "try { a(); } catch (err) { throw err; }", 0],
    [
      "a bindingless catch",
      "try { a(); } catch { show(t('common.errorNoCause')); }",
      0,
    ],
    [
      "an unrelated .message outside any catch",
      "const problem = read(); show(problem.message);",
      0,
    ],
    [
      "the correct spelling of a result's error",
      "show(problemMessageOf(download.error, t));",
      0,
    ],
    [
      "a same-named identifier that is not the binding",
      "try { a(); } catch (err) { const other = read(); show(other.message); }",
      0,
    ],
  ];

  for (const [name, source, expected] of cases) {
    it(`sees ${name}`, () => {
      expect(
        disclosuresIn("planted.tsx", parseSource("planted.tsx", source)),
      ).toHaveLength(expected);
    });
  }
});

describe("the caught-error-coverage rule", () => {
  it("reports a planted leak through its own corpus and visit", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text = "try { a(); } catch (e) { show(e.message); }\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "message src/features/planted.tsx:1",
    ]);
  });

  it("leaves test files out of its corpus", () => {
    const path = join(srcDir, "features", "planted.test.tsx");
    const text = "try { a(); } catch (e) { show(e.message); }\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([]);
  });
});
