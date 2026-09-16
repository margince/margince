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
 * TWO READINGS, because each covers the other's hole and the union can only
 * over-recognise — which fails loudly, where missing a selector fails silently
 * and lets a cleanup delete the class under a browser journey.
 *
 * By POSITION: the first argument of a call that takes a selector. That reads
 * `button.auto-row` and `div > .x`, which no shape rule can tell from a
 * property access. By SHAPE: a string opening with `.`, `#` or `[` and carrying
 * nothing but selector punctuation. That reads a selector handed to a helper of
 * the suite's own — `present(page, ".record-tabs")` — which no list of APIs
 * names.
 *
 * A template is read for its whole tokens, dropping the one touching an
 * interpolation: `` `.auto-row[data-id="${id}"]` `` names `.auto-row` and the
 * attribute is nobody's class.
 */
function selectorClassesIn(source: ts.SourceFile): string[] {
  const out: string[] = [];
  // `openEnded` says the piece is followed by an interpolation, so a name
  // running to its very end is half a name: `` `.row-${tone}` `` names no class
  // this can look up, and reading `row-` would report one nothing declares.
  const read = (text: string, openEnded = false) => {
    for (const found of text.matchAll(/\.([a-z][\w-]*)/g)) {
      if (openEnded && (found.index ?? 0) + found[0].length === text.length) {
        continue;
      }
      out.push(found[1]);
    }
  };
  // A string in selector POSITION is read whatever it looks like; one anywhere
  // else has to look like a selector.
  const readSelector = (node: ts.Node, positional: boolean) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      if (positional || looksLikeSelector(node.text)) {
        read(node.text);
      }
      return;
    }
    if (ts.isTemplateExpression(node)) {
      if (!positional && !looksLikeSelector(node.head.text)) {
        return;
      }
      read(node.head.text, true);
      node.templateSpans.forEach((span, index) => {
        read(span.literal.text, index < node.templateSpans.length - 1);
      });
    }
  };
  const visit = (node: ts.Node) => {
    if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const callee = ts.isPropertyAccessExpression(node.expression)
        ? node.expression.name.text
        : node.expression.getText(source);
      if (SELECTOR_CALLS.has(callee)) {
        for (const argument of node.arguments) {
          readSelector(argument, true);
        }
      }
    }
    readSelector(node, false);
    ts.forEachChild(node, visit);
  };
  visit(source);
  // A string in selector position that also LOOKS like one is read twice, which
  // is the two readings meeting rather than two selectors.
  return [...new Set(out)];
}

/**
 * The calls that take a CSS selector.
 *
 * Playwright's own, the DOM's, and the two this tree's suites wrap them in. A
 * name added here widens what is read; the shape rule beside it is what keeps a
 * name NOT here from going unread.
 */
const SELECTOR_CALLS = new Set([
  "locator",
  "querySelector",
  "querySelectorAll",
  "closest",
  "matches",
  "$",
  "$$",
  "$eval",
  "$$eval",
  "present",
  "edge",
  "topOf",
]);

/**
 * A string that is nothing but selector: punctuation, names, and spaces.
 *
 * One more thing is asked of it than the charset, because an `accept` attribute
 * is a comma-separated list of file extensions and reads as a selector
 * otherwise: `".txt,.md,.srt"`. So a shape-recognised selector has to carry a
 * character only a selector uses, or a hyphenated class name — which is what
 * every class in this tree is. A hyphen-free class in a bare list is still read
 * where it matters, by POSITION, because that is a selector handed to a call.
 */
const SELECTOR_SHAPE = /^[.#[][\w.#[\]='"\s>+~:,()-]*$/;
const SELECTOR_ONLY = /[#[\]>+~:\s]|\.[a-z]\w*-/;

function looksLikeSelector(text: string): boolean {
  return SELECTOR_SHAPE.test(text) && SELECTOR_ONLY.test(text);
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
  const bound = bindingsIn(source);
  const following = new Set<ts.Node>();
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
      return;
    }
    // A LOCAL BINDING IS FOLLOWED. `const classes = [...].join(" ")` and then
    // `className={classes}` is the ordinary way a component with three or four
    // conditional classes is written, and a reader that stopped at the
    // identifier recorded nothing for it — an orphan in one of those was
    // invisible. Followed ONCE per name, because a binding that refers to
    // itself would otherwise be walked forever.
    if (ts.isIdentifier(node)) {
      const initializer = bound.get(node.text);
      if (initializer && !following.has(initializer)) {
        following.add(initializer);
        value(initializer);
        following.delete(initializer);
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

/**
 * Every `const` in the module, by name, with what it was assigned.
 *
 * Read once per file rather than resolved through the checker: what a class
 * list is assembled from is nearly always a literal in the same module, and a
 * name shadowed in an inner scope resolves to whichever the walk saw last —
 * which over-reads rather than under-reads, the direction this gate is allowed
 * to be wrong in.
 */
function bindingsIn(source: ts.SourceFile): Map<string, ts.Expression> {
  const out = new Map<string, ts.Expression>();
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    ) {
      out.set(node.name.text, node.initializer);
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
    // WALKED ON PURPOSE. A selector for a class nothing declares is normally a
    // page carrying a name for a suite's sake — but not always, and a register
    // is where the exceptions say which they are. It only shrinks.
    const onPurpose = new Map<string, string>([
      [
        "e2e/contact-network.spec.ts walks .pn-ring",
        "the retired drawing, asserted ABSENT: `toHaveCount(0)` over classes " +
          "nothing declares is the point of that case rather than a mistake in it",
      ],
      [
        "e2e/contact-network.spec.ts walks .pn-node",
        "the same retired drawing",
      ],
      [
        "e2e/contact-network.spec.ts walks .pn-edge",
        "the same retired drawing",
      ],
      [
        "e2e/company-record.spec.ts walks .co-standing",
        "the class the company header retired in 1af81e93a, still walked by a " +
          "suite CI never runs — it is skipped without a live BASE_URL, so the " +
          "case is red for whoever next runs it. Repointing needs a stack to " +
          "confirm against: issue 5732",
      ],
    ]);
    for (const suite of suites) {
      const source = parseSource(suite, readFileSync(suite, "utf8"));
      // READ AS A SYNTAX TREE, for the reason the census above is: the two
      // leaks a text scan produced here were both prose — "`[].every()`" and
      // "a `.cf-count`" — written in comments explaining the very cases this
      // asks about.
      for (const name of selectorClassesIn(source)) {
        const walked = `${relative(frontendRoot, suite)} walks .${name}`;
        if (!declared.has(name) && !onPurpose.has(walked)) {
          pinned.push(walked);
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
  // half-read. `tone-` is not a class and `tone-warning` is not one this can know.
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

  // The ordinary way a component with three or four conditional classes is
  // written. A reader that stopped at the identifier recorded nothing for it,
  // so an orphan assembled this way was invisible.
  it("follows a class list assembled into a local binding", () => {
    expect(
      names(
        'const classes = ["card", open && "card-open"].filter(Boolean).join(" ");\n' +
          "<p className={classes}>x</p>",
      ),
    ).toEqual(["card", "card-open"]);
  });

  // And a binding that refers to itself is walked once rather than forever.
  it("follows a self-referring binding without looping", () => {
    expect(names('const a = [a, "row"];\n<p className={a}>x</p>')).toEqual([
      "row",
    ]);
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

// AND THE SELECTOR READER, asked about the forms a browser journey uses.
//
// The two readings cover each other's holes: a tag-qualified selector
// (`button.auto-row`) has no shape a rule can tell from a property access, and
// a selector handed to a helper of the suite's own is in no list of APIs.
describe("what a browser journey can be shown to walk", () => {
  const walks = (code: string) =>
    selectorClassesIn(parseSource("probe.spec.ts", code));

  it("reads a selector by the call that takes it", () => {
    expect(walks('page.locator("button.auto-row")')).toEqual(["auto-row"]);
    expect(walks('present(page, ".record-tabs")')).toEqual(["record-tabs"]);
  });

  it("reads a selector by its shape, wherever it is handed", () => {
    expect(walks('const sel = ".co-tabs .recordtabs-tab";')).toEqual([
      "co-tabs",
      "recordtabs-tab",
    ]);
  });

  it("reads a template's whole tokens and not its interpolation", () => {
    expect(walks('page.locator(`.auto-row[data-id="${id}"]`)')).toEqual([
      "auto-row",
    ]);
  });

  // An `accept` attribute is a comma-separated list of file extensions, and it
  // is nothing but selector characters. Reading it would name five classes no
  // sheet declares and no page carries.
  it("reads a list of file extensions as what it is", () => {
    expect(
      walks('expect(input).toHaveAttribute("accept", ".md,.srt,.pdf")'),
    ).toEqual([]);
  });

  it("reads nothing out of a property access", () => {
    expect(walks("page.locator")).toEqual([]);
  });
});
