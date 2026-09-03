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
 * Identifiers bound to a screen id, so a link written through a constant counts.
 *
 * `navigate({ screen: SCHEDULED_SCREEN })` is a door, and a gate reading only
 * quoted ids would call that screen unreached and be wrong in the one direction
 * a census must never fail: quietly, over a smaller subject than it claims.
 */
function screenAliases(
  sources: ReadonlyArray<readonly [string, string]>,
  screens: ReadonlySet<string>,
): Map<string, string> {
  const out = new Map<string, string>();
  for (const [path, source] of sources) {
    const walk = (node: ts.Node): void => {
      if (
        ts.isVariableDeclaration(node) &&
        ts.isIdentifier(node.name) &&
        node.initializer
      ) {
        const value = literal(node.initializer);
        if (value !== null && screens.has(value)) {
          out.set(node.name.text, value);
        }
      }
      ts.forEachChild(node, walk);
    };
    walk(parse(path, source));
  }
  return out;
}

/**
 * The screens this module links to.
 *
 * Two spellings, because the product uses both: an `#/…` address in markup, and
 * a `screen:` property handed to `navigate` or held as a route. The property is
 * read wherever it appears rather than only inside a call named `navigate` — a
 * gate that named its subject's spellings would be defeated by a rename, which
 * is the mistake this tree's gates keep being written to avoid.
 */
function linksIn(
  path: string,
  source: string,
  screens: ReadonlySet<string>,
  aliases: ReadonlyMap<string, string>,
): Set<string> {
  const found = new Set<string>();
  const address = (text: string): void => {
    for (const match of text.matchAll(/#\/([a-z-]+)/g)) {
      const id = match[1];
      if (id !== undefined && screens.has(id)) {
        found.add(id);
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
      ts.isPropertyAssignment(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "screen"
    ) {
      const value = literal(node.initializer);
      if (value !== null && screens.has(value)) {
        found.add(value);
      }
      if (ts.isIdentifier(node.initializer)) {
        const aliased = aliases.get(node.initializer.text);
        if (aliased !== undefined) {
          found.add(aliased);
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parse(path, source));
  return found;
}

describe("every screen has a door", () => {
  const screens: ReadonlySet<Screen> = new Set(SCREENS);
  const sources = shippedSources().map(
    (path) => [path, readFileSync(path, "utf8")] as const,
  );

  it("reads the whole shipped tree", () => {
    // Fail closed. A walk pointed at the wrong tree reports PASS over nothing,
    // and under-recognition is the one way a gate must not break.
    expect(sources.length).toBeGreaterThan(100);
  });

  it("reads router.tsx's own list, so the subject cannot drift from it", () => {
    const declared = waiversIn(readFileSync(routerModule, "utf8"));
    // The waivers are read from source and the subject is imported. If those
    // two ever describe different lists, every reason below is being applied to
    // a screen that may not be the one it was written for.
    expect([...declared.keys()].sort()).toEqual([...SCREENS].sort());
  });

  it("names no screen nothing can reach", () => {
    const waivers = waiversIn(readFileSync(routerModule, "utf8"));
    const aliases = screenAliases(sources, screens);
    const linked = new Set<string>();
    for (const [path, source] of sources) {
      for (const id of linksIn(path, source, screens, aliases)) {
        linked.add(id);
      }
    }
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
  });

  // The gate's own census. It reads a smaller tree than it claims, or matches a
  // shape the real defect does not take, and it reports PASS either way.
  it("sees a screen with no door, in every spelling a door takes", () => {
    const screens = new Set(["planted", "other"]);
    const linkedBy = (source: string) =>
      linksIn(join(srcRoot, "planted.ts"), source, screens, new Map());

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

  it("follows a door written through a constant", () => {
    const screens = new Set(["planted"]);
    const alias = `const PLANTED_SCREEN = "planted";`;
    const aliases = screenAliases(
      [[join(srcRoot, "a.ts"), alias] as const],
      screens,
    );

    expect(aliases.get("PLANTED_SCREEN")).toBe("planted");
    expect(
      linksIn(
        join(srcRoot, "b.ts"),
        `navigate({ screen: PLANTED_SCREEN });`,
        screens,
        aliases,
      ),
    ).toContain("planted");
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
