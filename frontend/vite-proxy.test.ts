// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import spaConfig from "./vite.config";

// Every origin-relative address the product HANDS SOMEBODY has to answer on the
// app's own port.
//
// The dev server is the app's origin, and it proxies a fixed list of prefixes;
// the api is a separate port a reader never sees. A connector screen that
// builds a copy-paste command from `location.origin` — which is right on a
// deployed stack, where the two are one host — therefore hands a developer a
// command that 404s while the endpoint answers correctly one port over. The
// reader goes looking for a misconfigured connector, a wrong secret, or a
// disabled extension. A broken example is worse than no example.
//
// So the prefixes are DERIVED from what the code builds rather than listed
// twice: a second connector publishing its own edge is covered the day it is
// written, instead of the day somebody remembers this file.

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..");

// pathTemplate matches a template literal that OPENS with an interpolation —
// `` `${anything}/x/…` `` — and answers the segment after it, which is what a
// proxy entry is keyed on.
//
// Anchored on the backtick, so only the ROOT interpolation counts. Without it
// every later `${…}/segment` in the same literal matched too, and
// `${location.origin}/v1/offers/${offer.id}/pdf` was read as an address at
// /pdf — a prefix nothing serves, reported against a line that is correct.
const pathTemplate = /`\$\{[^}]*\}\/([a-z0-9._-]+)/g;

// namedOrigin narrows the same shape to a root interpolation that SAYS origin.
const namedOrigin = /`\$\{[^}]*\borigin\b[^}]*\}\/([a-z0-9._-]+)/gi;

// relativeImport answers the specifiers a module pulls from beside itself.
const relativeImport = /\bfrom\s+"(\.[^"]*)"/g;

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "dist" || entry.startsWith(".")) {
      continue;
    }
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      out.push(...sourceFiles(path));
      continue;
    }
    if (/\.(ts|tsx)$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
      out.push(path);
    }
  }
  return out;
}

// The keys vite-pwa.ts reads too: the one list of what the api owns on this origin.
function proxiedPrefixes(): string[] {
  const keys = Object.keys(spaConfig.server?.proxy ?? {});
  // The floor: a reading that found nothing would report a clean pass over an
  // empty set.
  expect(keys.length).toBeGreaterThan(4);
  return keys;
}

// launcherPrefixes reads the `apiPrefixes` literal in desktop/launcher/web.go,
// refusing any element that is not a plain string rather than skipping it.
function launcherPrefixes(): string[] {
  const source = readFileSync(
    join(repo, "desktop", "launcher", "web.go"),
    "utf8",
  );
  const literal = /\bvar apiPrefixes = \[\]string\{([^}]*)\}/.exec(source);
  if (literal === null) {
    throw new Error(
      "desktop/launcher/web.go declares no `var apiPrefixes = []string{…}` for this test to read",
    );
  }
  const elements = literal[1]
    .replace(/\/\/[^\n]*/g, "")
    .split(",")
    .map((element) => element.trim())
    .filter((element) => element.length > 0);
  for (const element of elements) {
    expect(
      element,
      "an apiPrefixes element in desktop/launcher/web.go is not a string literal this test can read",
    ).toMatch(/^"[^"\\]*"$/);
  }
  return elements.map((element) => element.slice(1, -1));
}

// neighbourhood answers the files an address could be assembled across: the one
// reading `location.origin`, and the modules beside it that it imports.
//
// Because the READER of the origin and the BUILDER of the path are routinely
// two files. openchannel's screen takes `globalThis.location.origin` into a
// local and hands it to `inboundUrl(origin, endpoint)`; the template lives in
// contract.ts, one import away. A scan keyed on the word `origin` finds that
// one only because the parameter happens to be spelled so — rename it to `base`
// and /webhooks drops out of the census silently, which is the exact regression
// this file exists to catch.
function neighbourhood(file: string): string[] {
  const text = readFileSync(file, "utf8");
  const out = [file];
  for (const match of text.matchAll(relativeImport)) {
    const base = resolve(dirname(file), match[1]);
    for (const candidate of [
      `${base}.ts`,
      `${base}.tsx`,
      join(base, "index.ts"),
    ]) {
      try {
        if (statSync(candidate).isFile()) out.push(candidate);
      } catch {
        // A specifier this resolver cannot place is not a finding: it resolves
        // through tsconfig paths or a package, and neither assembles an address
        // beside the file that reads the origin.
      }
    }
  }
  return out;
}

describe("the dev server's proxy", () => {
  it("answers every origin-relative address the product hands somebody", () => {
    const proxied = new Set(proxiedPrefixes());

    const files = [
      ...sourceFiles(join(repo, "frontend", "src")),
      ...sourceFiles(join(repo, "extensions")),
    ];

    // TWO READINGS, unioned, because neither is sufficient alone.
    //
    // The first is the broad one: any template rooted at something SAYING
    // origin, wherever it sits. The second is the one that does not depend on
    // that word — every path template in the neighbourhood of a file that
    // actually reads `location.origin`. The second is why renaming a parameter
    // cannot quietly shrink this census.
    const built = new Map<string, string>();
    const byNeighbourhood = new Set<string>();
    for (const file of files) {
      const text = readFileSync(file, "utf8");
      for (const match of text.matchAll(namedOrigin)) {
        built.set(match[1], file.slice(repo.length + 1));
      }
      if (!/\blocation\.origin\b/.test(text)) continue;
      for (const near of neighbourhood(file)) {
        for (const match of readFileSync(near, "utf8").matchAll(pathTemplate)) {
          built.set(match[1], near.slice(repo.length + 1));
          byNeighbourhood.add(match[1]);
        }
      }
    }
    expect(built.size).toBeGreaterThan(0);
    // And the second reading found something of its own. Without this the union
    // above would go on passing after the neighbourhood walk broke, carried
    // entirely by the arm that reads the word.
    expect(
      byNeighbourhood.size,
      "no address was found through a file reading location.origin — the reading that does " +
        "not depend on an identifier being named `origin` has stopped reaching anything",
    ).toBeGreaterThan(0);

    for (const [prefix, file] of built) {
      expect(
        proxied.has(`/${prefix}`),
        `${file} builds an address at /${prefix} from the app's own origin, which the dev ` +
          `server does not proxy — the command it hands a reader 404s on the app's port while ` +
          `the endpoint answers one port over. Add "/${prefix}" to the proxy list in vite.config.ts.`,
      ).toBe(true);
    }
  });

  it("is the list the desktop launcher proxies, in both directions", () => {
    expect(
      launcherPrefixes().sort(),
      "desktop/launcher/web.go apiPrefixes and the proxy in vite.config.ts disagree: a path in " +
        "one and not the other reaches the api under `pnpm dev` and 404s in the desktop app, or " +
        "the reverse. Make the two lists the same.",
    ).toEqual(proxiedPrefixes().sort());
  });
});
