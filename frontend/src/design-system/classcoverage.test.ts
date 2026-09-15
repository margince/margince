// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  extensionLayers,
  filesMatching,
  filesUnder,
  parseSource,
} from "../../scripts/lib/source-tree";
import { withoutComments } from "../testing/css";

// A CLASS ON AN ELEMENT MATCHES A RULE SOMEWHERE, OR IT IS A LIE.
//
// A class name reads as intent. Review reads the intent, the reviewer agrees
// with it, and the page draws something else — nothing fails, because a
// selector nobody wrote is indistinguishable at review time from one that is
// there. `cell-stack` was rendered by three components and matched no rule
// anywhere, so two spans meant to sit on separate lines stayed inline and an
// operator diagnosing a stalled model lane read `04/09/2026, 15:29provider_quota`
// as one word (#5494).
//
// Eighty-one more were carrying the same lie when this gate was written.
//
// Under-recognition is the one way this must not break — a census that reads a
// smaller tree, reports PASS, and leaves no failing assertion to notice. So
// every corpus is derived from the tree and floored, the markup is read as a
// SYNTAX TREE rather than as text (a class name inside a comment is not a class
// name, and the scan that found these first counted one), and the one thing it
// cannot resolve is named below rather than passed over.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");
const extensionsRoot = join(frontendRoot, "..", "extensions");

function fromFrontend(path: string): string {
  return relative(frontendRoot, path).replaceAll("\\", "/");
}

/**
 * Every stylesheet the app loads, core and extension tier alike.
 *
 * A miss here is the expensive direction in reverse: a sheet the walk does not
 * reach makes every class it declares look orphaned, and the gate then reports
 * two thousand findings nobody can act on. The floor below is what says the
 * walk found the tree rather than a corner of it.
 */
function sheets(): string[] {
  const units = extensionLayers(extensionsRoot).flatMap((layer) =>
    filesMatching(layer, /\.css$/),
  );
  return filesMatching(sourceRoot, /\.css$/)
    .concat(units)
    .map(fromFrontend);
}

/**
 * The modules that draw the app. Tests, stories and testkits are not among
 * them: a class named in a test is an assertion about what a component emits,
 * and a story's fixture markup is scaffolding for one frame.
 */
function modules(): string[] {
  return filesUnder(sourceRoot)
    .concat(extensionFrontendFiles(extensionsRoot))
    .map(fromFrontend)
    .filter((where) => /\.tsx$/.test(where))
    .filter((where) => !/\.(test|stories)\.tsx$/.test(where))
    .filter((where) => !where.includes(".testkit."));
}

/**
 * Every class name a selector mentions, anywhere in a sheet.
 *
 * Read from the SELECTORS only — the text before each `{` — so a class named
 * in a declaration's value (a `content` string, a custom property holding a
 * name) does not count as declared. Comments are blanked first: a rule
 * explained in prose and then deleted leaves its name in the paragraph above
 * the gap.
 */
/**
 * The class names a suite's selector strings walk.
 *
 * A SELECTOR IS RECOGNISED BY ITS SHAPE rather than by the call around it:
 * Playwright reaches one through `locator`, `querySelector`, `$$eval` and a
 * handful of each suite's own helpers, and a gate that listed those would miss
 * the next one. A selector opens with `.`, `#` or `[` and carries nothing but
 * selector punctuation, which is what keeps `page.locator` — a property access
 * that happens to contain a dot — from reading as one.
 */
function selectorClassesIn(source: ts.SourceFile): string[] {
  const out: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      if (/^[.#[][\w.#[\]='"\s>+~:()-]*$/.test(node.text)) {
        for (const found of node.text.matchAll(/\.([a-z][\w-]*)/g)) {
          out.push(found[1]);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

function declaredClasses(): Set<string> {
  const out = new Set<string>();
  for (const sheet of sheets()) {
    for (const name of declaredIn(
      readFileSync(join(frontendRoot, sheet), "utf8"),
    )) {
      out.add(name);
    }
  }
  return out;
}

function declaredIn(css: string): Set<string> {
  const out = new Set<string>();
  for (const chunk of withoutComments(css).split("{")) {
    // THE SELECTOR ONLY, which is the text after the previous rule closed.
    // Splitting on `{` leaves the body of one rule in front of the next
    // selector, and a body holds dotted text of its own: `url("./icons/a.svg")`
    // would declare `icons` and `svg`. That widens what counts as declared,
    // which is the direction this gate must not fail in — a class genuinely
    // named `svg` would then pass as styled by a sheet that draws nothing of
    // the kind.
    const selector = chunk.slice(chunk.lastIndexOf("}") + 1);
    for (const found of selector.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)) {
      out.add(found[1]);
    }
  }
  return out;
}

/** One class name a module puts on an element, and where. */
type Rendered = { name: string; where: string; line: number };

/**
 * The class names a `className` attribute can be shown to produce.
 *
 * Read as VALUES rather than as every string in the subtree, which is the
 * difference between auditing a class list and auditing the code around it.
 * `` `lt-arrow${state === "asc" ? " up" : ""}` `` produces `lt-arrow` and
 * sometimes `up`; `asc` is a column state being compared, and a gate that
 * counted it would report a class nobody wrote. So a conditional contributes
 * its two branches and not its question, a comparison contributes nothing, and
 * a template's interpolations are not descended into at all.
 *
 * What IS read: literals, both branches of a conditional, both sides of `&&`,
 * `||`, `??` and `+`, every argument of a call (`cx("row", open && "row-open")`)
 * and every element of an array that is joined into one. The ordinary dynamic
 * class list is covered rather than waved past.
 *
 * THE ONE BLIND SPOT is a name whose own text is computed — the `tone-` of
 * `` `tone-${level}` ``. It is not a class this can look up, and guessing at the
 * variants would make the gate report names that exist and miss names that do
 * not. So the token touching an interpolation is dropped rather than half-read,
 * and what remains of such a template — every whole token in it — is still
 * checked. The base class of a variant pair is nearly always one of those, so
 * the shape the blind spot hides is a suffix on a base this gate has seen.
 */
function renderedIn(source: ts.SourceFile, where: string): Rendered[] {
  const out: Rendered[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
  const add = (text: string, node: ts.Node) => {
    for (const name of text.split(/\s+/).filter(Boolean)) {
      out.push({ name, where, line: at(node) });
    }
  };
  const value = (node: ts.Node): void => {
    if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node)) {
      value(node.expression);
      return;
    }
    if (ts.isJsxExpression(node)) {
      if (node.expression) {
        value(node.expression);
      }
      return;
    }
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      add(node.text, node);
      return;
    }
    if (ts.isTemplateExpression(node)) {
      // `head` runs up to the first `${`, and each span's literal runs from one
      // interpolation to the next. A piece flush against an interpolation ends
      // in a PREFIX rather than a name, or begins with a suffix — both are
      // dropped, leaving the whole tokens between them.
      add(whole(node.head.text, false, true), node.head);
      node.templateSpans.forEach((span, index) => {
        const last = index === node.templateSpans.length - 1;
        add(whole(span.literal.text, true, !last), span.literal);
      });
      return;
    }
    if (ts.isConditionalExpression(node)) {
      value(node.whenTrue);
      value(node.whenFalse);
      return;
    }
    if (ts.isBinaryExpression(node)) {
      const joins =
        node.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken ||
        node.operatorToken.kind === ts.SyntaxKind.BarBarToken ||
        node.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken ||
        node.operatorToken.kind === ts.SyntaxKind.PlusToken;
      if (joins) {
        value(node.left);
        value(node.right);
      }
      return;
    }
    if (ts.isCallExpression(node)) {
      for (const argument of node.arguments) {
        value(argument);
      }
      // `[…].filter(Boolean).join(" ")` — the list is the callee's subject
      // rather than an argument, and it is the list that holds the names.
      if (ts.isPropertyAccessExpression(node.expression)) {
        value(node.expression.expression);
      }
      return;
    }
    if (ts.isArrayLiteralExpression(node)) {
      for (const element of node.elements) {
        value(element);
      }
    }
  };
  const visit = (node: ts.Node) => {
    if (
      ts.isJsxAttribute(node) &&
      node.name.getText() === "className" &&
      node.initializer
    ) {
      value(node.initializer);
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/** A template piece with the partial token at either flush end removed. */
function whole(text: string, dropFirst: boolean, dropLast: boolean): string {
  const tokens = text.split(/\s+/);
  if (dropFirst && !/^\s/.test(text)) {
    tokens.shift();
  }
  if (dropLast && !/\s$/.test(text)) {
    tokens.pop();
  }
  return tokens.join(" ");
}

/**
 * A literal that reads as a class list rather than as words for a reader.
 *
 * A `className` may carry a sentence — an aria string interpolated by mistake,
 * a fragment of copy — and a gate that split one into "class names" would
 * report every word of it. The same shape badge-spelling.test.ts uses.
 */
function isClassToken(name: string): boolean {
  return /^-?[_a-zA-Z][\w-]*$/.test(name);
}

// THE DETECTOR, ASKED ABOUT EVERY SHAPE IT HAS TO READ.
//
// The census above can only report what this resolves, so a reader that quietly
// stopped seeing one shape would take the whole gate down to PASS with nothing
// failing. Each case below is a shape the tree actually writes.
// AND NOTHING SELECTS A CLASS NO SHEET DECLARES.
//
// The other direction, and the one that bit while this was being written. A
// suite that pins itself to a class name turns that name into a handle the page
// has to keep — so the class is no longer dead, it is dead-and-load-bearing,
// and the census above would have it deleted. Three Playwright journeys were
// walking `.record-tabs`, `.co-tabs` and `.worklist-row-decision`, none of
// which any sheet has ever drawn; removing them broke eleven browser cases
// eleven minutes into CI rather than here.
//
// A handle a page does not style is a `data-testid`. It says what it is for,
// nothing styles it by accident, and a stylist reading the markup is not told a
// rule exists.
describe("no suite pins itself to a class that styles nothing", () => {
  it("finds every selector the browser journeys walk", () => {
    const suites = filesMatching(join(frontendRoot, "e2e"), /\.ts$/);
    expect(
      suites.length,
      "no browser suites were found, so this arm read nothing",
    ).toBeGreaterThan(5);

    const declared = declaredClasses();
    const pinned: string[] = [];
    for (const suite of suites) {
      const source = parseSource(suite, readFileSync(suite, "utf8"));
      // READ AS A SYNTAX TREE, for the reason the census above is: the two
      // leaks a text scan produced here were both prose — "`[].every()`" and
      // "a `.cf-count`" — written in comments explaining the very cases this
      // asks about.
      for (const name of selectorClassesIn(source)) {
        if (!declared.has(name)) {
          pinned.push(`${relative(frontendRoot, suite)} walks .${name}`);
        }
      }
    }

    expect(
      [...new Set(pinned)].sort(),
      "these suites walk a class no stylesheet declares, so the page is carrying " +
        "the name for the suite's sake alone — give the element a data-testid and " +
        "walk that instead",
    ).toEqual([]);
  });
});

describe("what a className can be shown to produce", () => {
  const names = (markup: string) =>
    renderedIn(
      parseSource("probe.tsx", `const x = ${markup};`),
      "probe.tsx",
    ).map((one) => one.name);

  it("reads a plain list", () => {
    expect(names('<p className="a b">x</p>')).toEqual(["a", "b"]);
  });

  it("reads both branches of a conditional and not its question", () => {
    expect(names('<p className={on === "asc" ? "up" : "down"}>x</p>')).toEqual([
      "up",
      "down",
    ]);
  });

  it("reads a guard's class and not the guard", () => {
    expect(names('<p className={open && "row-open"}>x</p>')).toEqual([
      "row-open",
    ]);
  });

  it("reads every argument of an assembled list", () => {
    expect(names('<p className={cx("row", open && "on")}>x</p>')).toEqual([
      "row",
      "on",
    ]);
    expect(
      names('<p className={["row", "on"].filter(Boolean).join(" ")}>x</p>'),
    ).toEqual(["row", "on"]);
  });

  // The blind spot, stated as behaviour: the whole tokens of a template are
  // read and the one touching the interpolation is dropped rather than
  // half-read. `tone-` is not a class and `tone-warn` is not one this can know.
  it("reads a template's whole tokens and drops the one it cannot finish", () => {
    expect(names("<p className={`card tone-${level}`}>x</p>")).toEqual([
      "card",
    ]);
    expect(names("<p className={`${prefix}-tail head`}>x</p>")).toEqual([
      "head",
    ]);
  });

  it("reads nothing out of an expression it cannot resolve", () => {
    expect(names("<p className={styles.row}>x</p>")).toEqual([]);
  });
});

describe("every class an element carries is declared by a sheet", () => {
  const sheetFiles = sheets();
  const moduleFiles = modules();

  // THE CORPORA ANNOUNCE THEMSELVES. A walk that found nothing reports the
  // same silence as one that found nothing wrong, and for this gate the two
  // fail in opposite directions: no sheets makes everything an orphan, and no
  // modules makes nothing one.
  it("reads the tree it audits", () => {
    expect(
      sheetFiles.length,
      "no stylesheets were found, so every class would read as orphaned",
    ).toBeGreaterThan(100);
    expect(
      moduleFiles.length,
      "no modules were found, so no class would be read at all",
    ).toBeGreaterThan(300);
  });

  it("finds no class that styles nothing", () => {
    const declared = declaredClasses();
    expect(
      declared.size,
      "the sheets parsed to almost no class names, so this gate is reading them wrong",
    ).toBeGreaterThan(1000);

    const orphans: string[] = [];
    for (const where of moduleFiles) {
      const path = join(frontendRoot, where);
      const source = parseSource(path, readFileSync(path, "utf8"));
      for (const rendered of renderedIn(source, where)) {
        if (!isClassToken(rendered.name) || declared.has(rendered.name)) {
          continue;
        }
        orphans.push(`${rendered.where}:${rendered.line} .${rendered.name}`);
      }
    }

    expect(
      [...new Set(orphans)].sort(),
      "these class names are put on elements and matched by no selector in any " +
        "stylesheet: each is a rule that should exist and does not, or a name " +
        "that should be deleted — and until one of the two happens the page " +
        "draws something other than what the markup says",
    ).toEqual([]);
  });
});

// THE DECLARED SET IS SELECTORS, NOT DECLARATIONS.
//
// Widening what counts as declared is the direction this gate must not fail
// in: it reads a bigger set of "styled" names, reports PASS, and leaves no
// failing assertion to notice. A rule body holds dotted text of its own — a
// `url()` most often — and splitting a sheet on `{` leaves one rule's body
// sitting in front of the next rule's selector.
describe("what a stylesheet declares", () => {
  it("reads the selector and not the rule body", () => {
    const declared = declaredIn(
      `.icon { background: url("./icons/arrow.svg"); }
       .real { color: red; }`,
    );

    expect([...declared].sort()).toEqual(["icon", "real"]);
  });

  it("reads a selector nested inside a rule or a breakpoint", () => {
    const declared = declaredIn(
      `@media (min-width: 40rem) { .wide { display: grid; } }
       .card { &.is-open { color: red; } }`,
    );

    expect([...declared].sort()).toEqual(["card", "is-open", "wide"]);
  });
});
