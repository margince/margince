// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// Fitness function for a caught failure reaching the screen in the SERVER's
// words instead of the reader's.
//
// `problemMessageOf` (screens/common.tsx) calls itself the one way a caught
// failure becomes words on a screen, and the reason is not tidiness: `httperr`
// composes a refusal's detail from `err.Error()`, and every producer of
// `permission_denied` wraps it with internals — `auth.Require` sends the RBAC
// object and the verb, so the raw detail is literally
// "person.update: permission denied". Rendering it hands the reader who was
// just refused the shape of the authority model that refused them.
// `problemMessageOf` replaces that arm with catalog copy; a caught error read
// directly skips it.
//
// The obligation is DERIVED: the subject set is every `catch` binding under
// src/, found by parsing, so a new screen is enrolled the moment it is written.
// There is no list of files to keep up to date, which matters because the last
// hand-kept list of these sites went stale twice — a site was recorded fixed
// while the file it lived in was being rewritten with the same defect back in.
//
// FIVE ARMS, each keyed on what hid a real copy rather than on the shape that
// was noticed first:
//
//   1. `.message` off the binding. What hid it: the `error instanceof Error`
//      guard in front of it reads as careful handling, not as a bypass.
//   2. `String(binding)`. What hid it: it is the FALLBACK half of that same
//      ternary, so a reviewer who accepts arm 1's fix stops reading. It is not
//      a lesser leak — `String(problemError)` is
//      "ProblemError: person.update: permission denied", the same sentence with
//      a class name in front. Five of the six sites this gate was written over
//      carried this arm too, and none of the three hand sweeps counted it.
//   3. `${binding}` in a template, and `binding.toString()`. The same coercion
//      with no `String` to grep for.
//   4. A local alias — `const failure = err` — because a gate that a rename
//      defeats is a gate that gets renamed around rather than fixed.
//   5. `<anything>.error.message` — the same disclosure with NO `catch` in
//      sight. A react-query result carries the throw as `.error`, and a screen
//      rendering `download.error.message` never writes a catch clause at all,
//      so the four arms above are structurally blind to it. It is the arm that
//      was missing when this gate first read zero: the tree still had two.
//
// WHAT THIS DOES NOT CATCH, deliberately: a binding handed to a function that
// coerces it out of sight (`setError(describe(err))`); a binding stored in
// state and read after the catch block closes; and a DESTRUCTURED read
// (`const { message } = err as Error`). Following any of them needs the type
// checker plus a rule about which helpers count as rendering, and this repo has
// already learned that a gate with fiddly rules gets worked around. The
// destructured form is named rather than left implied: it is one edit away from
// the shapes below, and a reader who assumes it is covered would be wrong. The
// five arms above cover every shape that has actually shipped here.
//
// A console sink is not an exception today because none exists: the one
// sanctioned path to the console is `logUnexpectedError`, which is handed the
// error itself and never its text. If a legitimate console read ever appears,
// widen this with the reason written beside it — the arms are the gate, and an
// exception list is where a gate quietly stops holding.

const screensDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(screensDir, "..");

/** A rendered read of one caught binding: `<arm> <file>:<line>`. */
function disclosuresIn(fileName: string, text: string): string[] {
  const source = ts.createSourceFile(
    fileName,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const found: string[] = [];

  const visitCatch = (clause: ts.CatchClause) => {
    const declared = clause.variableDeclaration?.name;
    if (!declared || !ts.isIdentifier(declared)) {
      return;
    }
    // Arm 4: the binding plus every local alias of it, grown as the block is
    // walked. Declarations precede their uses, so one pass suffices.
    const names = new Set([declared.text]);

    // A cast, a parenthesis and a non-null assertion all leave the same value
    // in place — `(err as Error).message` is arm 1 wearing a cast.
    const unwrap = (node: ts.Expression): ts.Expression => {
      let inner = node;
      while (
        ts.isParenthesizedExpression(inner) ||
        ts.isAsExpression(inner) ||
        ts.isNonNullExpression(inner) ||
        ts.isTypeAssertionExpression(inner)
      ) {
        inner = inner.expression;
      }
      return inner;
    };
    const isBinding = (node: ts.Expression): boolean => {
      const inner = unwrap(node);
      return ts.isIdentifier(inner) && names.has(inner.text);
    };

    const at = (node: ts.Node) =>
      source.getLineAndCharacterOfPosition(node.getStart()).line + 1;

    const scan = (node: ts.Node) => {
      if (
        ts.isVariableDeclaration(node) &&
        ts.isIdentifier(node.name) &&
        node.initializer &&
        isBinding(node.initializer)
      ) {
        names.add(node.name.text);
      }
      if (ts.isPropertyAccessExpression(node) && isBinding(node.expression)) {
        if (node.name.text === "message") {
          found.push(`message ${fileName}:${at(node)}`);
        }
        if (node.name.text === "toString") {
          found.push(`toString ${fileName}:${at(node)}`);
        }
      }
      // `err["message"]` is the same read through the other accessor, and it
      // is the one a gate written against the dot form never sees.
      if (
        ts.isElementAccessExpression(node) &&
        isBinding(node.expression) &&
        ts.isStringLiteral(node.argumentExpression) &&
        node.argumentExpression.text === "message"
      ) {
        found.push(`message ${fileName}:${at(node)}`);
      }
      if (
        ts.isCallExpression(node) &&
        ts.isIdentifier(node.expression) &&
        node.expression.text === "String" &&
        node.arguments.length === 1 &&
        isBinding(node.arguments[0])
      ) {
        found.push(`String ${fileName}:${at(node)}`);
      }
      if (ts.isTemplateSpan(node) && isBinding(node.expression)) {
        found.push(`template ${fileName}:${at(node)}`);
      }
      ts.forEachChild(node, scan);
    };
    scan(clause.block);
  };

  // Arm 5, which needs no catch clause and therefore sits outside visitCatch.
  // `X.error` is where every react-query result files the throw, and 309 sites
  // in this tree read it through `problemMessageOf`. Reading `.message` off it
  // is the same leak by a route with no `catch` to anchor on.
  // A cast, a parenthesis or a non-null assertion between `.error` and
  // `.message` leaves the same read in place. `download.error!.message` is the
  // likely spelling, because strict-null narrowing is what invites the `!` —
  // so the arm has to see through it or it is blind to its own commonest form.
  const beneathWrappers = (node: ts.Expression): ts.Expression => {
    let inner = node;
    while (
      ts.isParenthesizedExpression(inner) ||
      ts.isAsExpression(inner) ||
      ts.isNonNullExpression(inner) ||
      ts.isTypeAssertionExpression(inner)
    ) {
      inner = inner.expression;
    }
    return inner;
  };
  // Either accessor on either hop, because there are four spellings of one
  // read and a rule written against the dot form sees one of them:
  // `r.error.message`, `r["error"].message`, `r.error["message"]`, and both
  // brackets. The census's first draft saw only the first.
  const namesProperty = (node: ts.Node, property: string): boolean => {
    if (ts.isPropertyAccessExpression(node)) {
      return node.name.text === property;
    }
    return (
      ts.isElementAccessExpression(node) &&
      ts.isStringLiteral(node.argumentExpression) &&
      node.argumentExpression.text === property
    );
  };
  const accessed = (node: ts.Node): ts.Expression | undefined =>
    ts.isPropertyAccessExpression(node) || ts.isElementAccessExpression(node)
      ? node.expression
      : undefined;
  const visitResultError = (node: ts.Node) => {
    const target = accessed(node);
    const beneath = target === undefined ? undefined : beneathWrappers(target);
    if (
      namesProperty(node, "message") &&
      beneath !== undefined &&
      namesProperty(beneath, "error")
    ) {
      const { line } = source.getLineAndCharacterOfPosition(node.getStart());
      found.push(`result.error ${fileName}:${line + 1}`);
    }
    ts.forEachChild(node, visitResultError);
  };

  const visit = (node: ts.Node) => {
    if (ts.isCatchClause(node)) {
      visitCatch(node);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  visitResultError(source);
  return found;
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sourceFiles(path);
    }
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)
      ? [path]
      : [];
  });
}

function disclosures(): string[] {
  // Only files that can contain a catch clause at all — the parse cost then
  // grows with the number of error handlers rather than with the SPA.
  const files = sourceFiles(srcRoot).filter((file) => {
    const text = readFileSync(file, "utf8");
    // Arms 1-4 need a catch clause; arm 5 needs none, so a file with only a
    // result-error read still has to be parsed. Narrowing on "catch" alone is
    // how arm 5 would have been added and still seen nothing — and narrowing on
    // ".error" alone is the same mistake one level down, since `r["error"]`
    // contains no dot before the word.
    return text.includes("catch") || text.includes("error");
  });
  // A sweep that found no files and a tree with no defects look identical from
  // the outside, and both look green.
  expect(files.length).toBeGreaterThan(0);
  return files
    .flatMap((file) =>
      disclosuresIn(relative(srcRoot, file), readFileSync(file, "utf8")),
    )
    .sort();
}

const PARSE_MS = 60_000;

describe("a caught failure", () => {
  it(
    "never reaches the screen in the server's own words",
    () => {
      const found = disclosures();
      expect(found, `\n${found.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );
});

// The gate's own defect test. Every case below is a shape that shipped here or
// is one edit away from one; a census of zero cannot tell a clean tree from a
// blind detector, so the detector is shown finding each shape and leaving the
// correct spelling alone.
describe("the census", () => {
  const cases: ReadonlyArray<readonly [string, string, number]> = [
    [
      "the ternary that shipped at four sites",
      "try { a(); } catch (err) { show(err instanceof Error ? err.message : String(err)); }",
      // Both halves of one ternary: `.message` AND the `String` fallback.
      2,
    ],
    ["a bare .message", "try { a(); } catch (e) { show(e.message); }", 1],
    [
      "a cast in front of it",
      "try { a(); } catch (e) { show((e as Error).message); }",
      1,
    ],
    ["a non-null assertion", "try { a(); } catch (e) { show(e!.message); }", 1],
    ["String() alone", "try { a(); } catch (e) { show(String(e)); }", 1],
    ["toString()", "try { a(); } catch (e) { show(e.toString()); }", 1],
    [
      "a template",
      // A template literal, so the planted case carries the real `${…}`
      // the gate has to see rather than a string that merely mentions one.
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
      // Found by a reviewer probing the first draft: the same read through the
      // other accessor, invisible to a rule written against the dot form.
      "a bracketed read of the same property",
      'try { a(); } catch (e) { show(e["message"]); }',
      1,
    ],
    [
      // Found by a second reviewer probing the widened arm 5: the bracket
      // spelling on the FIRST hop, which the `.error` prefilter also dropped.
      "a result error reached by bracket",
      'show(download["error"].message);',
      1,
    ],
    ["both hops by bracket", 'show(save["error"]["message"]);', 1],
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
    // The correct spellings, which must stay invisible to every arm.
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
      expect(disclosuresIn("planted.tsx", source)).toHaveLength(expected);
    });
  }
});
