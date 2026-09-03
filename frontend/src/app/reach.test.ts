// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  scriptKindFor,
} from "../../scripts/lib/source-tree";
import { NAV } from "./nav";
import { SCREENS, type Screen } from "./router";

// A capability nobody can navigate to is, from a user's seat, indistinguishable
// from one that was never built.
//
// Four shipped in this tree over a few weeks without a door: the AI verdict
// surface, the contract renewal verbs, the Worklist's personal scoping, and the
// Worklist page itself — which `router.tsx` registered, `nav.ts` did not carry,
// and nothing in the app linked. Each was built carefully. Reach was simply the
// last step, and nothing failed when it was missed.
//
// So it is a gate rather than a habit. A screen the router answers is reachable
// if the primary nav carries it, or if some shipped surface links to it. A
// screen that is genuinely reached from OUTSIDE the app says so in `router.tsx`,
// beside its own entry, with a reason.
//
// THE SUBJECT IS DERIVED. `SCREENS` and `NAV` are imported from the modules
// that own them, so a screen added tomorrow is judged tomorrow; the waivers are
// read out of `router.tsx`'s own source, where the next author adding a line to
// that list reads its neighbours' reasons.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");
const frontendRoot = resolve(srcRoot, "..");
const extensionsDir = resolve(frontendRoot, "..", "extensions");
const routerModule = join(here, "router.tsx");
const thisGate = fileURLToPath(import.meta.url);

/**
 * The waiver, spelled the way this tree's other opt-outs are.
 *
 * A reason is required for the reason `craft:ignore` and `plural-rule:allow`
 * require one: a bare marker is a keystroke that turns the gate off, and a gate
 * with one of those is decoration.
 */
const WAIVER = /reach:external[ \t]+(?<reason>\S.*)/;

/** A shipped module. Tests, stories and fixtures link to nothing a reader can press. */
function shippedSources(): string[] {
  return [...filesUnder(srcRoot), ...extensionFrontendFiles(extensionsDir)]
    .filter(
      (path) =>
        path !== thisGate &&
        path !== routerModule &&
        !/\.(test|testkit|stories|fixtures)\./.test(path),
    )
    .sort();
}

function parse(path: string, source: string): ts.SourceFile {
  return ts.createSourceFile(
    path,
    source,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(path),
  );
}

/**
 * The string this expression is, where it is written as one.
 *
 * `as const` and a parenthesis are unwrapped, because a constant naming a
 * screen is usually written `= "scheduled" as const` — and a reader that
 * stopped at the assertion would call that screen unreached while the palette
 * and the composer both navigate to it.
 */
function literal(node: ts.Node): string | null {
  if (ts.isAsExpression(node) || ts.isParenthesizedExpression(node)) {
    return literal(node.expression);
  }
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    return node.text;
  }
  return null;
}

/**
 * The screen ids named in `router.tsx`'s own list, with any reason waiving one.
 *
 * Read off the AST rather than by line arithmetic, for the reason the plural
 * gate gives: the formatter owns where lines break, and a marker matched by
 * counting lines comes undone silently the next time anything is reflowed.
 */
function waiversIn(source: string): Map<string, string | null> {
  const parsed = parse("router.tsx", source);
  const out = new Map<string, string | null>();
  const walk = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "SCREENS"
    ) {
      const array = node.initializer;
      const elements =
        array && ts.isAsExpression(array) ? array.expression : array;
      if (elements && ts.isArrayLiteralExpression(elements)) {
        for (const element of elements.elements) {
          const id = literal(element);
          if (id === null) {
            continue;
          }
          const comments =
            ts.getLeadingCommentRanges(source, element.getFullStart()) ?? [];
          const waiver = comments
            .map((range) => WAIVER.exec(source.slice(range.pos, range.end)))
            .find((match) => match !== null);
          out.set(id, waiver?.groups?.reason?.trim() ?? null);
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return out;
}

/**
 * What one module contributes: the doors it opens, and the constants it binds.
 *
 * Both in ONE walk, because the corpus is the whole shipped tree and the cost
 * of this gate is the parse. Two passes meant parsing every file twice, which
 * is what took it past its budget on a loaded runner while passing locally.
 *
 * A door has two spellings, because the product uses both: an `#/…` address in
 * markup, and a `screen:` property handed to `navigate` or held as a route. The
 * property is read wherever it appears rather than only inside a call named
 * `navigate` — a gate that named its subject's spellings would be defeated by a
 * rename, which is the mistake this tree's gates keep being written to avoid.
 *
 * A `screen:` written as an IDENTIFIER cannot be resolved yet: the constant may
 * be declared in a file this walk has not reached. So the name travels out as a
 * name, and `doorsIn` resolves it once every alias is known. Resolving inline
 * would make the answer depend on the order the files were read, which is the
 * quiet half of a census failing short.
 */
function readModule(
  path: string,
  source: string,
  screens: ReadonlySet<string>,
): Readonly<{
  linked: Set<string>;
  viaName: Set<string>;
  aliases: Map<string, string>;
}> {
  const linked = new Set<string>();
  const viaName = new Set<string>();
  const aliases = new Map<string, string>();
  const address = (text: string): void => {
    for (const match of text.matchAll(/#\/([a-z-]+)/g)) {
      const id = match[1];
      if (id !== undefined && screens.has(id)) {
        linked.add(id);
      }
    }
  };
  const walk = (node: ts.Node): void => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      address(node.text);
    }
    if (ts.isTemplateExpression(node)) {
      // Every literal chunk, not just the head. `${origin}/#/room?c=${…}` puts
      // the address in a SPAN, and a reader that stopped at the head would
      // call that screen unreached while the product mints links to it.
      address(node.head.text);
      for (const span of node.templateSpans) {
        address(span.literal.text);
      }
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    ) {
      const value = literal(node.initializer);
      if (value !== null && screens.has(value)) {
        aliases.set(node.name.text, value);
      }
    }
    if (
      ts.isPropertyAssignment(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "screen"
    ) {
      const value = literal(node.initializer);
      if (value !== null && screens.has(value)) {
        linked.add(value);
      }
      if (ts.isIdentifier(node.initializer)) {
        viaName.add(node.initializer.text);
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parse(path, source));
  return { linked, viaName, aliases };
}

/** Every screen the shipped tree opens a door to. */
function doorsIn(
  sources: ReadonlyArray<readonly [string, string]>,
  screens: ReadonlySet<string>,
): Set<string> {
  const linked = new Set<string>();
  const viaName = new Set<string>();
  const aliases = new Map<string, string>();
  for (const [path, source] of sources) {
    const read = readModule(path, source, screens);
    for (const id of read.linked) {
      linked.add(id);
    }
    for (const name of read.viaName) {
      viaName.add(name);
    }
    for (const [name, id] of read.aliases) {
      aliases.set(name, id);
    }
  }
  // Resolved last, against every alias the tree declares — so a door written
  // through a constant counts wherever that constant is declared.
  for (const name of viaName) {
    const id = aliases.get(name);
    if (id !== undefined) {
      linked.add(id);
    }
  }
  return linked;
}

/**
 * This gate's own ceiling, for the reason `one-plural-rule.test.ts` states at
 * length: the default timeout is arithmetic over WAITING, and this test waits
 * for nothing. Its cost is the CORPUS — it type-parses every shipped module in
 * the tree — and the two numbers have no relationship. It passed locally in
 * about a second and died at `Test timed out in 10437ms` on a CI runner
 * saturated by the rest of the suite.
 *
 * Derived per FILE and multiplied out, not picked whole, and written as the
 * product so the derivation is in the code: `scripts/test-budget.test.ts`
 * refuses a ceiling it cannot statically fold, correctly, since one no reader
 * can evaluate is one no reader can audit.
 *
 * 40ms a file is an order of magnitude over the local measurement, matching the
 * allowance the plural gate arrived at empirically on the same corpus with the
 * same parser. 1400 files is that gate's budgeted corpus too, chosen against
 * this tree's merge rate; the corpus is asserted in the body rather than read
 * into the constant, so outgrowing it fails by name and count instead of
 * returning as an opaque timeout.
 */
const PARSE_BUDGET_PER_FILE_MS = 40;
const BUDGETED_CORPUS_FILES = 1_400;
const SCAN_TIMEOUT_MS = BUDGETED_CORPUS_FILES * PARSE_BUDGET_PER_FILE_MS;

describe("every screen has a door", () => {
  const screens: ReadonlySet<Screen> = new Set(SCREENS);
  const sources = shippedSources().map(
    (path) => [path, readFileSync(path, "utf8")] as const,
  );

  it("reads the whole shipped tree", () => {
    // Fail closed. A walk pointed at the wrong tree reports PASS over nothing,
    // and under-recognition is the one way a gate must not break.
    expect(sources.length).toBeGreaterThan(100);
    // And fail LOUDLY when the tree outgrows the ceiling's premise, which the
    // ceiling itself cannot do: it is a literal because the budget gate has to
    // read it, so it cannot scale with the corpus it is a budget for.
    expect(sources.length).toBeLessThanOrEqual(BUDGETED_CORPUS_FILES);
  });

  it("reads router.tsx's own list, so the subject cannot drift from it", () => {
    const declared = waiversIn(readFileSync(routerModule, "utf8"));
    // The waivers are read from source and the subject is imported. If those
    // two ever describe different lists, every reason below is being applied to
    // a screen that may not be the one it was written for.
    expect([...declared.keys()].sort()).toEqual([...SCREENS].sort());
  });

  it(
    "names no screen nothing can reach",
    () => {
      const waivers = waiversIn(readFileSync(routerModule, "utf8"));
      const linked = doorsIn(sources, screens);
      const carried = new Set(NAV.map((item) => item.screen));

      const unreached = [...SCREENS].filter(
        (screen) =>
          !carried.has(screen) &&
          !linked.has(screen) &&
          // A reason is required. A waiver without one is not a waiver.
          !waivers.get(screen),
      );

      expect(
        unreached,
        "A screen the router answers that the nav does not carry and nothing " +
          "links to is reachable only by typing its address. Give it a nav row " +
          "or a link — or, if it is genuinely opened from outside the app, say " +
          "so beside its entry in router.tsx: `// reach:external <reason>`.",
      ).toEqual([]);
    },
    SCAN_TIMEOUT_MS,
  );

  // The gate's own census. It reads a smaller tree than it claims, or matches a
  // shape the real defect does not take, and it reports PASS either way.
  it("sees a screen with no door, in every spelling a door takes", () => {
    const screens = new Set(["planted", "other"]);
    const linkedBy = (source: string) =>
      doorsIn([[join(srcRoot, "planted.ts"), source] as const], screens);

    expect(linkedBy(`const href = "#/planted";`)).toContain("planted");
    expect(linkedBy(`const href = \`#/planted/\${id}\`;`)).toContain("planted");
    // The address in a template SPAN rather than its head, which is what a
    // link built from an origin looks like.
    expect(
      linkedBy(`const href = \`\${origin}/#/planted?c=\${token}\`;`),
    ).toContain("planted");
    expect(linkedBy(`navigate({ screen: "planted" });`)).toContain("planted");
    // A route object held rather than navigated to — the palette's shape.
    expect(linkedBy(`const row = { route: { screen: "planted" } };`)).toContain(
      "planted",
    );
    // And nothing at all for a module that merely mentions the word.
    expect(linkedBy(`const label = "planted";`).size).toBe(0);
    // Membership in a set of screens is not a door. `RAIL_LESS_SCREENS` lists
    // ids to describe how they are FRAMED, and reading that as a link would
    // excuse exactly the screens most likely to have no door.
    expect(
      linkedBy(`const railLess = new Set(["planted", "other"]);`).size,
    ).toBe(0);
  });

  // The constant is declared in ONE file and used in ANOTHER, in that order and
  // in the reverse, because a walk that resolved a name the moment it met it
  // would answer differently depending on which file it read first — and the
  // wrong answer is the quiet one: the screen reported as having no door.
  it("follows a door written through a constant, whichever file comes first", () => {
    const screens = new Set(["planted"]);
    const declares = [
      join(srcRoot, "a.ts"),
      `export const PLANTED_SCREEN = "planted";`,
    ] as const;
    const uses = [
      join(srcRoot, "b.ts"),
      `navigate({ screen: PLANTED_SCREEN });`,
    ] as const;

    expect(doorsIn([declares, uses], screens)).toContain("planted");
    expect(doorsIn([uses, declares], screens)).toContain("planted");
    // And a name nothing binds to a screen is not a door.
    expect(doorsIn([uses], screens).size).toBe(0);
  });

  it("does not accept a waiver with no reason", () => {
    const waived = waiversIn(
      [
        `export const SCREENS = [`,
        `  // reach:external opened by the browser extension, never linked here`,
        `  "client",`,
        `  // reach:external`,
        `  "bare",`,
        `  "plain",`,
        `] as const;`,
      ].join("\n"),
    );

    expect(waived.get("client")).toBe(
      "opened by the browser extension, never linked here",
    );
    expect(waived.get("bare")).toBeNull();
    expect(waived.get("plain")).toBeNull();
  });
});
